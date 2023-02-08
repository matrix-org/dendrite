package routing

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/util"
	cache "github.com/patrickmn/go-cache"
)

type SharedSecretRegistrationRequest struct {
	User     string `json:"username"`
	Password string `json:"password"`
	Nonce    string `json:"nonce"`
	MacBytes []byte
	MacStr   string `json:"mac"`
	Admin    bool   `json:"admin"`
}

func NewSharedSecretRegistrationRequest(reader io.ReadCloser) (*SharedSecretRegistrationRequest, error) {
	defer internal.CloseAndLogIfError(context.Background(), reader, "NewSharedSecretRegistrationRequest: failed to close request body")
	var ssrr SharedSecretRegistrationRequest
	err := json.NewDecoder(reader).Decode(&ssrr)
	if err != nil {
		return nil, err
	}
	ssrr.MacBytes, err = hex.DecodeString(ssrr.MacStr)
	return &ssrr, err
}

type SharedSecretRegistration struct {
	sharedSecret string
	nonces       *cache.Cache
}

func NewSharedSecretRegistration(sharedSecret string) *SharedSecretRegistration {
	return &SharedSecretRegistration{
		sharedSecret: sharedSecret,
		// nonces live for 5mins, purge every 10mins
		nonces: cache.New(5*time.Minute, 10*time.Minute),
	}
}

func (r *SharedSecretRegistration) GenerateNonce() string {
	nonce := util.RandomString(16)
	r.nonces.Set(nonce, true, cache.DefaultExpiration)
	return nonce
}

func (r *SharedSecretRegistration) validNonce(nonce string) bool {
	_, exists := r.nonces.Get(nonce)
	return exists
}

func (r *SharedSecretRegistration) IsValidMacLogin(
	nonce, username, password string,
	isAdmin bool,
	givenMac []byte,
) (bool, error) {
	// Check that shared secret registration isn't disabled.
	if r.sharedSecret == "" {
		return false, errors.New("shared secret registration is disabled")
	}
	if !r.validNonce(nonce) {
		return false, fmt.Errorf("incorrect or expired nonce: %s", nonce)
	}

	// Check that username/password don't contain the HMAC delimiters.
	if strings.Contains(username, "\x00") {
		return false, errors.New("username contains invalid character")
	}
	if strings.Contains(password, "\x00") {
		return false, errors.New("password contains invalid character")
	}

	adminString := "notadmin"
	if isAdmin {
		adminString = "admin"
	}
	joined := strings.Join([]string{nonce, username, password, adminString}, "\x00")

	mac := hmac.New(sha1.New, []byte(r.sharedSecret))
	_, err := mac.Write([]byte(joined))
	if err != nil {
		return false, err
	}
	expectedMAC := mac.Sum(nil)

	return hmac.Equal(givenMac, expectedMAC), nil
}
