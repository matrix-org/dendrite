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
	"reflect"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func TestLoginFromJSONReader(t *testing.T) {
	ctx := context.Background()

	tsts := []struct {
		Name string
		Body string

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
			WantUsername: "alice",
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
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			var userAPI fakeUserInternalAPI
			cfg := &config.ClientAPI{
				Matrix: &config.Global{
					SigningIdentity: gomatrixserverlib.SigningIdentity{
						ServerName: serverName,
					},
				},
			}
			login, cleanup, err := LoginFromJSONReader(ctx, strings.NewReader(tst.Body), &userAPI, &userAPI, cfg)
			if err != nil {
				t.Fatalf("LoginFromJSONReader failed: %+v", err)
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
		Name string
		Body string

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
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			var userAPI fakeUserInternalAPI
			cfg := &config.ClientAPI{
				Matrix: &config.Global{
					SigningIdentity: gomatrixserverlib.SigningIdentity{
						ServerName: serverName,
					},
				},
			}
			_, cleanup, errRes := LoginFromJSONReader(ctx, strings.NewReader(tst.Body), &userAPI, &userAPI, cfg)
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
	res.Account = &uapi.Account{}
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
