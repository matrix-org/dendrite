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
	"database/sql"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type loginType string

// https://matrix.org/docs/spec/client_server/r0.3.0.html#login
const (
	PasswordBased loginType = "m.login.password"
	TokenBased    loginType = "m.login.token"
)

type loginFlows struct {
	Flows []flow `json:"flows"`
}

type flow struct {
	Type   string   `json:"type"`
	Stages []string `json:"stages"`
}

type loginRequest struct {
	Type               loginType `json:"type"`
	User               string    `json:"user"`
	Medium             string    `json:"medium"`
	Address            string    `json:"address"`
	Password           string    `json:"password"`
	Token              string    `json:"token"`
	DeviceID           string    `json:"device_id"`
	InitialDisplayName *string   `json:"initial_device_display_name"`
}

type loginResponse struct {
	UserID      string                       `json:"user_id"`
	AccessToken string                       `json:"access_token"`
	HomeServer  gomatrixserverlib.ServerName `json:"home_server"`
	DeviceID    string                       `json:"device_id"`
}

func defaultPasswordLogin() loginFlows {
	f := loginFlows{}
	s := flow{string(PasswordBased), []string{string(PasswordBased)}}
	f.Flows = append(f.Flows, s)
	return f
}

func handlePasswordLogin(
	r loginRequest, accountDB *accounts.Database, deviceDB *devices.Database,
	req *http.Request, cfg config.Dendrite) *util.JSONResponse {

	localpart := r.User
	acc, err := accountDB.GetAccountByPassword(req.Context(), localpart, r.Password)

	if err != nil {
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		return &util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("username or password was incorrect, or the account does not exist"),
		}
	}

	token, err := auth.GenerateAccessToken()
	if err != nil {
		httputil.LogThenError(req, err)
	}

	// TODO: Use the device ID in the request
	dev, err := deviceDB.CreateDevice(
		req.Context(), acc.Localpart, nil, token, r.InitialDisplayName,
	)
	if err != nil {
		return &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
		}
	}

	return &util.JSONResponse{
		Code: 200,
		JSON: loginResponse{
			UserID:      dev.UserID,
			AccessToken: dev.AccessToken,
			HomeServer:  cfg.Matrix.ServerName,
			DeviceID:    dev.ID,
		},
	}
}

func handleTokenLogin(
	r loginRequest, deviceDB *devices.Database,
	req *http.Request, cfg config.Dendrite) *util.JSONResponse {
	if r.Token == "" {
		return &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.MissingToken("missing access token"),
		}
	}

	dev, err := deviceDB.GetDeviceByAccessToken(req.Context(), r.Token)
	if err != nil {
		if err == sql.ErrNoRows {
			return &util.JSONResponse{
				Code: 401,
				JSON: jsonerror.UnknownToken("Unknown token"),
			}
		}
		return &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.Unknown("Unexpected Server error occurred"),
		}
	}

	if dev.ID != r.DeviceID {
		// The access token specified in the request was generated for a
		// different device.
		return &util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("The access token was generated for a different device."),
		}
	}

	return &util.JSONResponse{
		Code: 200,
		JSON: loginResponse{
			UserID:      dev.UserID,
			AccessToken: dev.AccessToken,
			HomeServer:  cfg.Matrix.ServerName,
			DeviceID:    dev.ID,
		},
	}
}

// Login implements GET and POST /login
func Login(
	req *http.Request, accountDB *accounts.Database, deviceDB *devices.Database,
	cfg config.Dendrite,
) util.JSONResponse {
	if req.Method == "GET" {
		return util.JSONResponse{
			Code: 200,
			JSON: defaultPasswordLogin(),
		}
	} else if req.Method == "POST" {
		var r loginRequest
		resErr := httputil.UnmarshalJSONRequest(req, &r)
		if resErr != nil {
			return *resErr
		}
		if r.User == "" {
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON("'user' must be supplied."),
			}
		}

		util.GetLogger(req.Context()).WithField("user", r.User).Info("Processing login request")

		if strings.HasPrefix(r.User, "@") {
			var domain gomatrixserverlib.ServerName
			var err error
			_, domain, err = gomatrixserverlib.SplitID('@', r.User)
			if err != nil {
				return util.JSONResponse{
					Code: 400,
					JSON: jsonerror.InvalidUsername("Invalid username"),
				}
			}

			if domain != cfg.Matrix.ServerName {
				return util.JSONResponse{
					Code: 400,
					JSON: jsonerror.InvalidUsername("User ID not ours"),
				}
			}
		}

		if r.Type == PasswordBased {
			return *handleTokenLogin(r, deviceDB, req, cfg)
		}
		return *handlePasswordLogin(r, accountDB, deviceDB, req, cfg)
	}
	return util.JSONResponse{
		Code: 405,
		JSON: jsonerror.NotFound("Bad method"),
	}
}
