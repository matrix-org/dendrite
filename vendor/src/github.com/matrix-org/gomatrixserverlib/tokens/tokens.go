// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tokens

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	macaroon "gopkg.in/macaroon.v2"
)

const (
	macaroonVersion = macaroon.V2
	defaultDuration = 2 * 60
	// UserPrefix is a common prefix for every user_id caveat
	UserPrefix = "user_id = "
	// TimePrefix is a common prefix for every expiry caveat
	TimePrefix = "time < "
	// Gen is a common caveat for every token
	Gen = "gen = 1"
)

// TokenOptions represent parameters of Token
type TokenOptions struct {
	ServerPrivateKey []byte `yaml:"private_key"`
	ServerName       string `yaml:"server_name"`
	UserID           string `json:"user_id"`
	Duration         int    // optional
}

// GenerateLoginToken generates a short term login token to be used as
// token authentication ("m.login.token")
func GenerateLoginToken(op TokenOptions) (string, error) {
	if !isValidTokenOptions(op) {
		return "", errors.New("The given TokenOptions is invalid")
	}

	mac, err := generateBaseMacaroon(op.ServerPrivateKey, op.ServerName, op.UserID)
	if err != nil {
		return "", err
	}

	if op.Duration == 0 {
		op.Duration = defaultDuration
	}
	now := time.Now().Second()
	expiryCaveat := TimePrefix + strconv.Itoa(now+op.Duration)
	err = mac.AddFirstPartyCaveat([]byte(expiryCaveat))
	if err != nil {
		return "", macaroonError(err)
	}

	urlSafeEncode, err := serializeMacaroon(*mac)
	if err != nil {
		return "", macaroonError(err)
	}
	return urlSafeEncode, nil
}

// isValidTokenOptions checks for required fields in a TokenOptions
func isValidTokenOptions(op TokenOptions) bool {
	if op.ServerPrivateKey == nil || op.ServerName == "" || op.UserID == "" {
		return false
	}
	return true
}

// generateBaseMacaroon generates a base macaroon common for accessToken & loginToken.
// Returns a macaroon tied with userID,
// returns an error if something goes wrong.
func generateBaseMacaroon(
	secret []byte, ServerName string, userID string,
) (*macaroon.Macaroon, error) {
	mac, err := macaroon.New(secret, []byte(userID), ServerName, macaroonVersion)
	if err != nil {
		return nil, macaroonError(err)
	}

	err = mac.AddFirstPartyCaveat([]byte(Gen))
	if err != nil {
		return nil, macaroonError(err)
	}

	err = mac.AddFirstPartyCaveat([]byte(UserPrefix + userID))
	if err != nil {
		return nil, macaroonError(err)
	}

	return mac, nil
}

func macaroonError(err error) error {
	return fmt.Errorf("Macaroon creation failed: %s", err.Error())
}

// serializeMacaroon takes a macaroon to be serialized.
// returns its base64 encoded string, URL safe, which can be sent via web, email, etc.
func serializeMacaroon(m macaroon.Macaroon) (string, error) {
	bin, err := m.MarshalBinary()
	if err != nil {
		return "", err
	}

	urlSafeEncode := base64.RawURLEncoding.EncodeToString(bin)
	return urlSafeEncode, nil
}

// deSerializeMacaroon takes a base64 encoded string of a macaroon to be de-serialized.
// Returns a macaroon. On failure returns error with description.
func deSerializeMacaroon(urlSafeEncode string) (macaroon.Macaroon, error) {
	var mac macaroon.Macaroon
	bin, err := base64.RawURLEncoding.DecodeString(urlSafeEncode)
	if err != nil {
		return mac, err
	}

	err = mac.UnmarshalBinary(bin)
	return mac, err
}
