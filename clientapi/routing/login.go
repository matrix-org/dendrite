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

	"context"
	"database/sql"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
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
	DeviceID           string  `json:"device_id"`
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
	if req.Method == http.MethodGet { // TODO: support other forms of login other than password, depending on config options
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: passwordLogin(),
		}
	} else if req.Method == http.MethodPost {
		var r passwordRequest
		resErr := httputil.UnmarshalJSONRequest(req, &r)
		if resErr != nil {
			return *resErr
		}
		if r.User == "" {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.BadJSON("'user' must be supplied."),
			}
		}

		util.GetLogger(req.Context()).WithField("user", r.User).Info("Processing login request")

		localpart, err := userutil.ParseUsernameParam(r.User, &cfg.Matrix.ServerName)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}

		acc, err := accountDB.GetAccountByPassword(req.Context(), localpart, r.Password)
		if err != nil {
			// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
			// but that would leak the existence of the user.
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden("username or password was incorrect, or the account does not exist"),
			}
		}

		token, err := auth.GenerateAccessToken()
		if err != nil {
			httputil.LogThenError(req, err)
		}

		dev, err := getDevice(req.Context(), r, deviceDB, acc, localpart, token)
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

// check if device exists else create one
func getDevice(
	ctx context.Context,
	r passwordRequest,
	deviceDB *devices.Database,
	acc *authtypes.Account,
	localpart, token string,
) (dev *authtypes.Device, err error) {
	dev, err = deviceDB.GetDeviceByID(ctx, localpart, r.DeviceID)
	if err == sql.ErrNoRows {
		// device doesn't exist, create one
		dev, err = deviceDB.CreateDevice(
			ctx, acc.Localpart, nil, token, r.InitialDisplayName,
		)
	}
	return
}
