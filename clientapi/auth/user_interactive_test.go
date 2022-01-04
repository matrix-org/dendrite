package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/tidwall/gjson"
	"testing"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

var (
	ctx        = context.Background()
	serverName = gomatrixserverlib.ServerName("example.com")
	// space separated localpart+password -> account
	lookup = make(map[string]*api.Account)
	device = &api.Device{
		AccessToken: "flibble",
		DisplayName: "My Device",
		ID:          "device_id_goes_here",
	}
	_, priv, _ = ed25519.GenerateKey(rand.Reader)
	pub64      = base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
)

func getAccountByPassword(ctx context.Context, localpart, plaintextPassword string) (*api.Account, error) {
	acc, ok := lookup[localpart+" "+plaintextPassword]
	if !ok {
		return nil, fmt.Errorf("unknown user/password")
	}
	return acc, nil
}

func getAccountByChallengeResponse(ctx context.Context, localpart, b64encodedSignature, challenge string) (*api.Account, error) {
	acc, ok := lookup[localpart+" "+pub64]
	if !ok {
		return nil, fmt.Errorf("unknown user/pubkey")
	}
	return acc, nil
}

func setup() *UserInteractive {
	cfg := &config.ClientAPI{
		Matrix: &config.Global{
			ServerName: serverName,
		},
	}
	return NewUserInteractive(getAccountByPassword, getAccountByChallengeResponse, cfg)
}

func TestUserInteractiveChallenge(t *testing.T) {
	uia := setup()
	// no auth key results in a challenge
	_, errRes := uia.Verify(ctx, []byte(`{}`), device)
	if errRes == nil {
		t.Fatalf("Verify succeeded with {} but expected failure")
	}
	if errRes.Code != 401 {
		t.Errorf("Expected HTTP 401, got %d", errRes.Code)
	}
}

func TestUserInteractivePasswordLogin(t *testing.T) {
	uia := setup()
	// valid password login succeeds when an account exists
	lookup["alice herpassword"] = &api.Account{
		Localpart:  "alice",
		ServerName: serverName,
		UserID:     fmt.Sprintf("@alice:%s", serverName),
	}
	// valid password requests
	testCases := []json.RawMessage{
		// deprecated form
		[]byte(`{
			"auth": {
				"type": "m.login.password",
				"user": "alice",
				"password": "herpassword"
			}
		}`),
		// new form
		[]byte(`{
			"auth": {
				"type": "m.login.password",
				"identifier": {
					"type": "m.id.user",
					"user": "alice"
				},
				"password": "herpassword"
			}
		}`),
	}
	for _, tc := range testCases {
		_, errRes := uia.Verify(ctx, tc, device)
		if errRes != nil {
			t.Errorf("Verify failed but expected success for request: %s - got %+v", string(tc), errRes)
		}
	}
}

func TestUserInteractivePasswordBadLogin(t *testing.T) {
	uia := setup()
	// password login fails when an account exists but is specced wrong
	lookup["bob hispassword"] = &api.Account{
		Localpart:  "bob",
		ServerName: serverName,
		UserID:     fmt.Sprintf("@bob:%s", serverName),
	}
	// invalid password requests
	testCases := []struct {
		body    json.RawMessage
		wantRes util.JSONResponse
	}{
		{
			// fields not in an auth dict
			body: []byte(`{
				"type": "m.login.password",
				"user": "bob",
				"password": "hispassword"
			}`),
			wantRes: util.JSONResponse{
				Code: 401,
			},
		},
		{
			// wrong type
			body: []byte(`{
				"auth": {
					"type": "m.login.not_password",
					"identifier": {
						"type": "m.id.user",
						"user": "bob"
					},
					"password": "hispassword"
				}
			}`),
			wantRes: util.JSONResponse{
				Code: 400,
			},
		},
		{
			// identifier type is wrong
			body: []byte(`{
				"auth": {
					"type": "m.login.password",
					"identifier": {
						"type": "m.id.thirdparty",
						"user": "bob"
					},
					"password": "hispassword"
				}
			}`),
			wantRes: util.JSONResponse{
				Code: 401,
			},
		},
		{
			// wrong password
			body: []byte(`{
				"auth": {
					"type": "m.login.password",
					"identifier": {
						"type": "m.id.user",
						"user": "bob"
					},
					"password": "not_his_password"
				}
			}`),
			wantRes: util.JSONResponse{
				Code: 401,
			},
		},
	}
	for _, tc := range testCases {
		_, errRes := uia.Verify(ctx, tc.body, device)
		if errRes == nil {
			t.Errorf("Verify succeeded but expected failure for request: %s", string(tc.body))
			continue
		}
		if errRes.Code != tc.wantRes.Code {
			t.Errorf("got code %d want code %d for request: %s", errRes.Code, tc.wantRes.Code, string(tc.body))
		}
	}
}

func TestUserInteractiveDigitalSignatureLogin(t *testing.T) {
	uia := setup()
	// valid digital signature login succeeds when an account exists
	lookup["alice "+pub64] = &api.Account{
		Localpart:  "alice",
		ServerName: serverName,
		UserID:     fmt.Sprintf("@alice:%s", serverName),
	}
	getSessionReq := `{
		"identifier": {
			"type": "m.id.user",
			"user": "alice"
		},
		"type": "m.login.challenge_response"
	}`
	_, errSessionRes := uia.Verify(ctx, []byte(getSessionReq), device)
	byteSessionRes, _ := json.Marshal(errSessionRes)
	var sessionRes struct {
		Session string            `json:"session"`
		Params  map[string]string `json:"params"`
	}
	jsonRes := gjson.GetBytes(byteSessionRes, "JSON").Raw
	_ = json.Unmarshal([]byte(jsonRes), &sessionRes)

	session := sessionRes.Session
	sig := ed25519.Sign(priv, []byte(sessionRes.Params["challenge"]))
	b64sig := base64.StdEncoding.EncodeToString(sig)
	loginReq := fmt.Sprintf(`{
		"auth": {
			"session": "%s",
			"type": "m.login.challenge_response",
			"signature": "%s",
			"identifier": {
				"type": "m.id.user",
				"user": "alice"}
			},
		"type": "m.login.challenge_response"
	}`, session, b64sig)

	_, err := uia.Verify(ctx, []byte(loginReq), device)
	if err != nil {
		t.Errorf("Verify failed but expected success for request: %s - got %+v", loginReq, err)
	}
}

func TestUserInteractiveInvalidDigitalSignatureLogin(t *testing.T) {
	uia := setup()
	// login fails when no sig is provided
	lookup["alice "+pub64] = &api.Account{
		Localpart:  "alice",
		ServerName: serverName,
		UserID:     fmt.Sprintf("@alice:%s", serverName),
	}
	getSessionReq := `{
		"identifier": {
			"type": "m.id.user",
			"user": "alice"
		},
		"type": "m.login.challenge_response"
	}`
	_, errSessionRes := uia.Verify(ctx, []byte(getSessionReq), device)
	byteSessionRes, _ := json.Marshal(errSessionRes)
	var sessionRes struct {
		Session string            `json:"session"`
		Params  map[string]string `json:"params"`
	}
	jsonRes := gjson.GetBytes(byteSessionRes, "JSON").Raw
	_ = json.Unmarshal([]byte(jsonRes), &sessionRes)

	session := sessionRes.Session
	loginReq := fmt.Sprintf(`{
		"auth": {
			"session": "%s",
			"type": "m.login.challenge_response",
			"signature": "",
			"identifier": {
				"type": "m.id.user",
				"user": "alice"}
			},
		"type": "m.login.challenge_response"
	}`, session)

	resp, err := uia.Verify(ctx, []byte(loginReq), device)
	if err == nil {
		t.Errorf("Verify succeeded but expected failure for request: %s - got %+v", loginReq, resp)
	}
}
