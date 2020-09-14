// Copyright 2017 Vector Creations Ltd
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

package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type loginResponse struct {
	UserID      string                       `json:"user_id"`
	AccessToken string                       `json:"access_token"`
	HomeServer  gomatrixserverlib.ServerName `json:"home_server"`
	DeviceID    string                       `json:"device_id"`
}

type flows struct {
	Flows []flow `json:"flows"`
}

type flow struct {
	Type string `json:"type"`
}

func passwordLogin() flows {
	f := flows{}
	s := flow{
		Type: "m.login.password",
	}
	f.Flows = append(f.Flows, s)
	return f
}

func ssoLogin() flows {
	f := flows{}
	s := flow{
		Type: "m.login.sso",
	}
	f.Flows = append(f.Flows, s)
	return f
}

// Login implements GET and POST /login
func Login(
	req *http.Request, accountDB accounts.Database, userAPI userapi.UserInternalAPI,
	cfg *config.ClientAPI,
) util.JSONResponse {
	if req.Method == http.MethodGet {
		// TODO: support other forms of login other than password, depending on config options
		flows := passwordLogin()
		if cfg.CAS.Enabled {
			flows.Flows = append(flows.Flows, ssoLogin().Flows...)
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: flows,
		}
	} else if req.Method == http.MethodPost {
		// TODO: is the the right way to read the body and re-add it?
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			// TODO: is this appropriate?
			return util.JSONResponse{
				Code: http.StatusMethodNotAllowed,
				JSON: jsonerror.NotFound("Bad method"),
			}
		}
		// add the body back to the request because ioutil.ReadAll consumes the body
		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		// marshall the body into an unstructured json map
		var jsonBody map[string]interface{}
		if err := json.Unmarshal([]byte(body), &jsonBody); err != nil {
			return util.JSONResponse{
				Code: http.StatusMethodNotAllowed,
				JSON: jsonerror.NotFound("Bad method"),
			}
		}

		loginType := jsonBody["type"].(string)
		if loginType == "m.login.password" {
			return doPasswordLogin(req, accountDB, userAPI, cfg)
		} else if loginType == "m.login.token" {
			return doTokenLogin(req, accountDB, userAPI, cfg)
		}
	}

	return util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

// Handles a m.login.password login type request
func doPasswordLogin(
	req *http.Request, accountDB accounts.Database, userAPI userapi.UserInternalAPI,
	cfg *config.ClientAPI,
) util.JSONResponse {
	typePassword := auth.LoginTypePassword{
		GetAccountByPassword: accountDB.GetAccountByPassword,
		Config:               cfg,
	}
	r := typePassword.Request()
	resErr := httputil.UnmarshalJSONRequest(req, r)
	if resErr != nil {
		return *resErr
	}
	login, authErr := typePassword.Login(req.Context(), r)
	if authErr != nil {
		return *authErr
	}

	// make a device/access token
	return completeAuth(req.Context(), cfg.Matrix.ServerName, userAPI, login)
}

// Handles a m.login.token login type request
func doTokenLogin(req *http.Request, accountDB accounts.Database, userAPI userapi.UserInternalAPI,
	cfg *config.ClientAPI,
) util.JSONResponse {
	// create a struct with the appropriate DB(postgres/sqlite) function and the configs
	typeToken := auth.LoginTypeToken{
		GetAccountByLocalpart: accountDB.GetAccountByLocalpart,
		Config:                cfg,
	}
	r := typeToken.Request()
	resErr := httputil.UnmarshalJSONRequest(req, r)
	if resErr != nil {
		return *resErr
	}
	login, authErr := typeToken.Login(req.Context(), r)
	if authErr != nil {
		return *authErr
	}

	// make a device/access token
	authResult := completeAuth(req.Context(), cfg.Matrix.ServerName, userAPI, login)

	// the login is successful, delete the login token before returning the access token to the client
	if authResult.Code == http.StatusOK {
		if err := auth.DeleteLoginToken(r.(*auth.LoginTokenRequest).Token); err != nil {
			// TODO: what to do here?
		}
	}
	return authResult
}

func completeAuth(
	ctx context.Context, serverName gomatrixserverlib.ServerName, userAPI userapi.UserInternalAPI, login *auth.Login,
) util.JSONResponse {
	token, err := auth.GenerateAccessToken()
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("auth.GenerateAccessToken failed")
		return jsonerror.InternalServerError()
	}

	localpart, err := userutil.ParseUsernameParam(login.Username(), &serverName)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("auth.ParseUsernameParam failed")
		return jsonerror.InternalServerError()
	}

	var performRes userapi.PerformDeviceCreationResponse
	err = userAPI.PerformDeviceCreation(ctx, &userapi.PerformDeviceCreationRequest{
		DeviceDisplayName: login.InitialDisplayName,
		DeviceID:          login.DeviceID,
		AccessToken:       token,
		Localpart:         localpart,
	}, &performRes)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: loginResponse{
			UserID:      performRes.Device.UserID,
			AccessToken: performRes.Device.AccessToken,
			HomeServer:  serverName,
			DeviceID:    performRes.Device.ID,
		},
	}
}
