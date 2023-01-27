// Copyright 2021 The Matrix.org Foundation C.I.C.
//
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

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

var cfg = &config.ClientAPI{
	Matrix: &config.Global{
		SigningIdentity: gomatrixserverlib.SigningIdentity{
			ServerName: serverName,
		},
	},
	Derived: &config.Derived{
		ApplicationServices: []config.ApplicationService{
			{
				ID:      "anapplicationservice",
				ASToken: "astoken",
				NamespaceMap: map[string][]config.ApplicationServiceNamespace{
					"users": {
						{
							Exclusive:    true,
							Regex:        "@alice:example.com",
							RegexpObject: regexp.MustCompile("@alice:example.com"),
						},
					},
				},
			},
		},
	},
}

func TestLoginFromJSONReader(t *testing.T) {
	ctx := context.Background()

	tsts := []struct {
		Name  string
		Body  string
		Token string

		WantUsername      string
		WantDeviceID      string
		WantDeletedTokens []string
	}{
		{
			Name: "passwordWorks",
			Body: `{
				"type": "m.login.password",
				"identifier": { "type": "m.id.user", "user": "alice" },
				"password": "herpassword",
				"device_id": "adevice"
            }`,
			WantUsername: "@alice:example.com",
			WantDeviceID: "adevice",
		},
		{
			Name: "tokenWorks",
			Body: `{
				"type": "m.login.token",
				"token": "atoken",
				"device_id": "adevice"
            }`,
			WantUsername:      "@auser:example.com",
			WantDeviceID:      "adevice",
			WantDeletedTokens: []string{"atoken"},
		},
		{
			Name: "appServiceWorksUserID",
			Body: `{
				"type": "m.login.application_service",
				"identifier": { "type": "m.id.user", "user": "@alice:example.com" },
				"device_id": "adevice"
			}`,
			Token: "astoken",

			WantUsername: "@alice:example.com",
			WantDeviceID: "adevice",
		},
		{
			Name: "appServiceWorksLocalpart",
			Body: `{
				"type": "m.login.application_service",
				"identifier": { "type": "m.id.user", "user": "alice" },
				"device_id": "adevice"
			}`,
			Token: "astoken",

			WantUsername: "alice",
			WantDeviceID: "adevice",
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			var userAPI fakeUserInternalAPI

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tst.Body))
			if tst.Token != "" {
				req.Header.Add("Authorization", "Bearer "+tst.Token)
			}

			login, cleanup, jsonErr := LoginFromJSONReader(req, &userAPI, &userAPI, cfg)
			if jsonErr != nil {
				t.Fatalf("LoginFromJSONReader failed: %+v", jsonErr)
			}

			cleanup(ctx, &util.JSONResponse{Code: http.StatusOK})

			if login.Username() != tst.WantUsername {
				t.Errorf("Username: got %q, want %q", login.Username(), tst.WantUsername)
			}

			if login.DeviceID == nil {
				if tst.WantDeviceID != "" {
					t.Errorf("DeviceID: got %v, want %q", login.DeviceID, tst.WantDeviceID)
				}
			} else {
				if *login.DeviceID != tst.WantDeviceID {
					t.Errorf("DeviceID: got %q, want %q", *login.DeviceID, tst.WantDeviceID)
				}
			}

			if !reflect.DeepEqual(userAPI.DeletedTokens, tst.WantDeletedTokens) {
				t.Errorf("DeletedTokens: got %+v, want %+v", userAPI.DeletedTokens, tst.WantDeletedTokens)
			}
		})
	}
}

func TestBadLoginFromJSONReader(t *testing.T) {
	ctx := context.Background()

	tsts := []struct {
		Name  string
		Body  string
		Token string

		WantErrCode string
	}{
		{Name: "empty", WantErrCode: "M_BAD_JSON"},
		{
			Name:        "badUnmarshal",
			Body:        `badsyntaxJSON`,
			WantErrCode: "M_BAD_JSON",
		},
		{
			Name: "badPassword",
			Body: `{
				"type": "m.login.password",
				"identifier": { "type": "m.id.user", "user": "alice" },
				"password": "invalidpassword",
				"device_id": "adevice"
            }`,
			WantErrCode: "M_FORBIDDEN",
		},
		{
			Name: "badToken",
			Body: `{
				"type": "m.login.token",
				"token": "invalidtoken",
				"device_id": "adevice"
            }`,
			WantErrCode: "M_FORBIDDEN",
		},
		{
			Name: "badType",
			Body: `{
				"type": "m.login.invalid",
				"device_id": "adevice"
            }`,
			WantErrCode: "M_INVALID_ARGUMENT_VALUE",
		},
		{
			Name: "noASToken",
			Body: `{
				"type": "m.login.application_service",
				"identifier": { "type": "m.id.user", "user": "@alice:example.com" },
				"device_id": "adevice"
			}`,
			WantErrCode: "M_MISSING_TOKEN",
		},
		{
			Name:  "badASToken",
			Token: "badastoken",
			Body: `{
				"type": "m.login.application_service",
				"identifier": { "type": "m.id.user", "user": "@alice:example.com" },
				"device_id": "adevice"
			}`,
			WantErrCode: "M_UNKNOWN_TOKEN",
		},
		{
			Name:  "badASNamespace",
			Token: "astoken",
			Body: `{
				"type": "m.login.application_service",
				"identifier": { "type": "m.id.user", "user": "@bob:example.com" },
				"device_id": "adevice"
			}`,
			WantErrCode: "M_EXCLUSIVE",
		},
		{
			Name:  "badASUserID",
			Token: "astoken",
			Body: `{
				"type": "m.login.application_service",
				"identifier": { "type": "m.id.user", "user": "@alice:wrong.example.com" },
				"device_id": "adevice"
			}`,
			WantErrCode: "M_INVALID_USERNAME",
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			var userAPI fakeUserInternalAPI

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tst.Body))
			if tst.Token != "" {
				req.Header.Add("Authorization", "Bearer "+tst.Token)
			}

			_, cleanup, errRes := LoginFromJSONReader(req, &userAPI, &userAPI, cfg)
			if errRes == nil {
				cleanup(ctx, nil)
				t.Fatalf("LoginFromJSONReader err: got %+v, want code %q", errRes, tst.WantErrCode)
			} else if merr, ok := errRes.JSON.(*jsonerror.MatrixError); ok && merr.ErrCode != tst.WantErrCode {
				t.Fatalf("LoginFromJSONReader err: got %+v, want code %q", errRes, tst.WantErrCode)
			}
		})
	}
}

type fakeUserInternalAPI struct {
	UserInternalAPIForLogin
	DeletedTokens []string
}

func (ua *fakeUserInternalAPI) QueryAccountByPassword(ctx context.Context, req *uapi.QueryAccountByPasswordRequest, res *uapi.QueryAccountByPasswordResponse) error {
	if req.PlaintextPassword == "invalidpassword" {
		res.Account = nil
		return nil
	}
	res.Exists = true
	res.Account = &uapi.Account{UserID: userutil.MakeUserID(req.Localpart, req.ServerName)}
	return nil
}

func (ua *fakeUserInternalAPI) PerformLoginTokenDeletion(ctx context.Context, req *uapi.PerformLoginTokenDeletionRequest, res *uapi.PerformLoginTokenDeletionResponse) error {
	ua.DeletedTokens = append(ua.DeletedTokens, req.Token)
	return nil
}

func (ua *fakeUserInternalAPI) PerformLoginTokenCreation(ctx context.Context, req *uapi.PerformLoginTokenCreationRequest, res *uapi.PerformLoginTokenCreationResponse) error {
	return nil
}

func (*fakeUserInternalAPI) QueryLoginToken(ctx context.Context, req *uapi.QueryLoginTokenRequest, res *uapi.QueryLoginTokenResponse) error {
	if req.Token == "invalidtoken" {
		return nil
	}

	res.Data = &uapi.LoginTokenData{UserID: "@auser:example.com"}
	return nil
}
