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

type loginFlows struct {
	Flows []flow `json:"flows"`
}

type flow struct {
	Type   string   `json:"type"`
	Stages []string `json:"stages"`
}

type passwordRequest struct {
	User               string  `json:"user"`
	Password           string  `json:"password"`
	InitialDisplayName *string `json:"initial_device_display_name"`
}

type loginResponse struct {
	UserID      string                       `json:"user_id"`
	AccessToken string                       `json:"access_token"`
	HomeServer  gomatrixserverlib.ServerName `json:"home_server"`
	DeviceID    string                       `json:"device_id"`
}

func passwordLogin() loginFlows {
	f := loginFlows{}
	s := flow{"m.login.password", []string{"m.login.password"}}
	f.Flows = append(f.Flows, s)
	return f
}

// Login implements GET and POST /login
func Login(
	req *http.Request, accountDB *accounts.Database, deviceDB *devices.Database,
	cfg config.Dendrite,
) util.JSONResponse {
	if req.Method == "GET" { // TODO: support other forms of login other than password, depending on config options
		return util.JSONResponse{
			Code: 200,
			JSON: passwordLogin(),
		}
	} else if req.Method == "POST" {
		var r passwordRequest
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

		// r.User can either be a user ID or just the localpart... or other things maybe.
		localpart := r.User
		if strings.HasPrefix(r.User, "@") {
			var domain gomatrixserverlib.ServerName
			var err error
			localpart, domain, err = gomatrixserverlib.SplitID('@', r.User)
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

		acc, err := accountDB.GetAccountByPassword(req.Context(), localpart, r.Password)
		if err != nil {
			// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
			// but that would leak the existence of the user.
			return util.JSONResponse{
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
			return util.JSONResponse{
				Code: 500,
				JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
			}
		}

		return util.JSONResponse{
			Code: 200,
			JSON: loginResponse{
				UserID:      dev.UserID,
				AccessToken: dev.AccessToken,
				HomeServer:  cfg.Matrix.ServerName,
				DeviceID:    dev.ID,
			},
		}
	}
	return util.JSONResponse{
		Code: 405,
		JSON: jsonerror.NotFound("Bad method"),
	}
}
