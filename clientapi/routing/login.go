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
	"encoding/json"
	"fmt"
	"net/http"

	"context"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/dendrite/userapi/storage/devices"
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

type loginIdentifier struct {
	Type string `json:"type"`
	User string `json:"user"`
}

type loginWithPasswordRequest struct {
	Identifier loginIdentifier `json:"identifier"`
	User       string          `json:"user"` // deprecated in favour of identifier
	Password   string          `json:"password"`
	// Both DeviceID and InitialDisplayName can be omitted, or empty strings ("")
	// Thus a pointer is needed to differentiate between the two
	InitialDisplayName *string `json:"initial_device_display_name"`
	DeviceID           *string `json:"device_id"`
}

type loginRequest struct {
	Type string `json:"type"`
	loginWithPasswordRequest
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
	req *http.Request, accountDB accounts.Database, deviceDB devices.Database,
	cfg *config.Dendrite,
) util.JSONResponse {
	if req.Method == http.MethodGet { // TODO: support other forms of login other than password, depending on config options
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: passwordLogin(),
		}
	} else if req.Method == http.MethodPost {
		var r passwordRequest
		var acc *api.Account
		var errJSON *util.JSONResponse
		resErr := httputil.UnmarshalJSONRequest(req, &r)
		if resErr != nil {
			return *resErr
		}

		j, _ := json.MarshalIndent(temp, "", "  ")
		fmt.Println(string(j))

		var r loginRequest
		json.Unmarshal(j, &r)

		switch r.Type {
		case "m.login.password":
			j, _ := json.MarshalIndent(r, "", "  ")
			fmt.Printf("LOGIN REQUEST: %+v\n", string(j))
			switch r.Identifier.Type {
			case "m.id.user":
				if r.Identifier.User == "" {
					return util.JSONResponse{
						Code: http.StatusBadRequest,
						JSON: jsonerror.BadJSON("'user' must be supplied."),
					}
				}
			}
			acc, errJSON = r.processUsernamePasswordLoginRequest(req, accountDB, cfg, r.Identifier.User)
			if errJSON != nil {
				return *errJSON
			}
		default:
			// TODO: The below behaviour is deprecated but without it Riot iOS won't log in
			if r.User != "" {
				acc, errJSON = r.processUsernamePasswordLoginRequest(req, accountDB, cfg, r.User)
				if errJSON != nil {
					return *errJSON
				}
			} else {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: jsonerror.BadJSON("login identifier '" + r.Identifier.Type + "' not supported"),
				}
			}
		}

		token, err := auth.GenerateAccessToken()
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("auth.GenerateAccessToken failed")
			return jsonerror.InternalServerError()
		}

		dev, err := getDevice(req.Context(), r.loginWithPasswordRequest, deviceDB, acc, token)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
			}
		}

		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: loginResponse{
				UserID:      dev.UserID,
				AccessToken: dev.AccessToken,
				HomeServer:  cfg.Matrix.ServerName,
				DeviceID:    dev.ID,
			},
		}
	}
	return util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

// getDevice returns a new or existing device
func getDevice(
	ctx context.Context,
	r loginWithPasswordRequest,
	deviceDB devices.Database,
	acc *api.Account,
	token string,
) (dev *api.Device, err error) {
	dev, err = deviceDB.CreateDevice(
		ctx, acc.Localpart, r.DeviceID, token, r.InitialDisplayName,
	)
	return
}

func (r *passwordRequest) processUsernamePasswordLoginRequest(
	req *http.Request, accountDB accounts.Database,
	cfg *config.Dendrite, username string,
) (acc *api.Account, errJSON *util.JSONResponse) {
	util.GetLogger(req.Context()).WithField("user", username).Info("Processing login request")

	localpart, err := userutil.ParseUsernameParam(username, &cfg.Matrix.ServerName)
	if err != nil {
		errJSON = &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
		return
	}

	acc, err = accountDB.GetAccountByPassword(req.Context(), localpart, r.Password)
	if err != nil {
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		errJSON = &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("username or password was incorrect, or the account does not exist"),
		}
		return
	}

	return
}
