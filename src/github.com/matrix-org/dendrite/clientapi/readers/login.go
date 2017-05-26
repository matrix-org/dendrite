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

package readers

import (
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
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
	User     string `json:"user"`
	Password string `json:"password"`
}

type loginResponse struct {
	UserID      string                       `json:"user_id"`
	AccessToken string                       `json:"access_token"`
	HomeServer  gomatrixserverlib.ServerName `json:"home_server"`
}

func passwordLogin() loginFlows {
	f := loginFlows{}
	s := flow{"m.login.password", []string{"m.login.password"}}
	f.Flows = append(f.Flows, s)
	return f
}

// Login implements GET and POST /login
func Login(req *http.Request, accountDB *accounts.Database, deviceDB *devices.Database, cfg config.ClientAPI) util.JSONResponse {
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

		acc, err := accountDB.GetAccountByPassword(r.User, r.Password)
		if err != nil {
			// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
			// but that would leak the existence of the user.
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.BadJSON("username or password was incorrect, or the account does not exist"),
			}
		}

		token, err := auth.GenerateAccessToken()
		if err != nil {
			return util.JSONResponse{
				Code: 500,
				JSON: jsonerror.Unknown("Failed to generate access token"),
			}
		}

		// TODO: Use the device ID in the request
		dev, err := deviceDB.CreateDevice(acc.Localpart, auth.UnknownDeviceID, token)
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
				HomeServer:  cfg.ServerName,
			},
		}
	}
	return util.JSONResponse{
		Code: 405,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

func makeUserID(localpart string, domain gomatrixserverlib.ServerName) string {
	return fmt.Sprintf("@%s:%s", localpart, domain)
}
