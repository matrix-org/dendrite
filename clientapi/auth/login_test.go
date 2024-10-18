// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/element-hq/dendrite/clientapi/userutil"
	"github.com/element-hq/dendrite/setup/config"
	uapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

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
			cfg := &config.ClientAPI{
				Matrix: &config.Global{
					SigningIdentity: fclient.SigningIdentity{
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

		WantErrCode spec.MatrixErrorCode
	}{
		{Name: "empty", WantErrCode: spec.ErrorBadJSON},
		{
			Name:        "badUnmarshal",
			Body:        `badsyntaxJSON`,
			WantErrCode: spec.ErrorBadJSON,
		},
		{
			Name: "badPassword",
			Body: `{
				"type": "m.login.password",
				"identifier": { "type": "m.id.user", "user": "alice" },
				"password": "invalidpassword",
				"device_id": "adevice"
            }`,
			WantErrCode: spec.ErrorForbidden,
		},
		{
			Name: "badToken",
			Body: `{
				"type": "m.login.token",
				"token": "invalidtoken",
				"device_id": "adevice"
            }`,
			WantErrCode: spec.ErrorForbidden,
		},
		{
			Name: "badType",
			Body: `{
				"type": "m.login.invalid",
				"device_id": "adevice"
            }`,
			WantErrCode: spec.ErrorInvalidParam,
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
			cfg := &config.ClientAPI{
				Matrix: &config.Global{
					SigningIdentity: fclient.SigningIdentity{
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
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tst.Body))
			if tst.Token != "" {
				req.Header.Add("Authorization", "Bearer "+tst.Token)
			}

			_, cleanup, errRes := LoginFromJSONReader(req, &userAPI, &userAPI, cfg)
			if errRes == nil {
				cleanup(ctx, nil)
				t.Fatalf("LoginFromJSONReader err: got %+v, want code %q", errRes, tst.WantErrCode)
			} else if merr, ok := errRes.JSON.(spec.MatrixError); ok && merr.ErrCode != tst.WantErrCode {
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
