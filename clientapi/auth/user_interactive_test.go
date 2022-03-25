package auth

import (
	"context"
	"encoding/json"
	"fmt"
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
)

type fakeAccountDatabase struct {
	api.UserAccountAPI
}

func (d *fakeAccountDatabase) PerformPasswordUpdate(ctx context.Context, req *api.PerformPasswordUpdateRequest, res *api.PerformPasswordUpdateResponse) error {
	return nil
}

func (d *fakeAccountDatabase) PerformAccountDeactivation(ctx context.Context, req *api.PerformAccountDeactivationRequest, res *api.PerformAccountDeactivationResponse) error {
	return nil
}

func (d *fakeAccountDatabase) QueryAccountByPassword(ctx context.Context, req *api.QueryAccountByPasswordRequest, res *api.QueryAccountByPasswordResponse) error {
	acc, ok := lookup[req.Localpart+" "+req.PlaintextPassword]
	if !ok {
		return fmt.Errorf("unknown user/password")
	}
	res.Account = acc
	res.Exists = true
	return nil
}

func setup() *UserInteractive {
	cfg := &config.ClientAPI{
		Matrix: &config.Global{
			ServerName: serverName,
		},
	}
	return NewUserInteractive(&fakeAccountDatabase{}, cfg)
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
