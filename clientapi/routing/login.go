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
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
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

func enabledLoginTypes(cfg *config.ClientAPI) flows {
	f := flows{}
	for _, loginType := range cfg.LoginTypes {
		s := flow{
			Type: loginType,
		}
		f.Flows = append(f.Flows, s)
	}
	return f
}

// Login implements GET and POST /login
func Login(
	req *http.Request, accountDB accounts.Database, userAPI userapi.UserInternalAPI,
	cfg *config.ClientAPI, userInteractiveAuth *auth.UserInteractive,
) util.JSONResponse {
	if req.Method == http.MethodGet {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: enabledLoginTypes(cfg),
		}
	} else if req.Method == http.MethodPost {
		loginType, err := httputil.GetLoginType(req)
		if err != nil {
			return *err
		}
		switch loginType {
		case authtypes.LoginTypePassword:
			typePassword := auth.LoginTypePassword{
				GetAccountByPassword: accountDB.GetAccountByPassword,
				Config:               cfg,
			}
			r := typePassword.Request()
			resErr := httputil.UnmarshalJSONRequest(req, r)
			if resErr != nil {
				return *resErr
			}
			login, authErr := typePassword.Login(req.Context(), r, "")
			if authErr != nil {
				return *authErr
			}
			// make a device/access token
			return completeAuth(req.Context(), cfg.Matrix.ServerName, userAPI, login, req.RemoteAddr, req.UserAgent())
		case authtypes.LoginTypeChallengeResponse:
			defer req.Body.Close() // nolint:errcheck
			bodyBytes, err := ioutil.ReadAll(req.Body)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: jsonerror.BadJSON("The request body could not be read: " + err.Error()),
				}
			}
			login, resErr := userInteractiveAuth.Verify(req.Context(), bodyBytes, nil)
			if resErr != nil {
				return *resErr
			}
			// create a device/access token
			return completeAuth(req.Context(), cfg.Matrix.ServerName, userAPI, login, req.RemoteAddr, req.UserAgent())
		default:
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidParam(fmt.Sprintf("Unsupported login type: %s", loginType)),
			}
		}
	}
	return util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

func completeAuth(
	ctx context.Context, serverName gomatrixserverlib.ServerName, userAPI userapi.UserInternalAPI, login *auth.Login,
	ipAddr, userAgent string,
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
		IPAddr:            ipAddr,
		UserAgent:         userAgent,
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
