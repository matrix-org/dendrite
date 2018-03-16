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
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/auth/tokens"
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

func defaultPasswordLogin() loginFlows {
	f := loginFlows{}
	s := flow{string(authtypes.LoginTypePassword), []string{string(authtypes.LoginTypePassword)}}
	f.Flows = append(f.Flows, s)
	return f
}

// GetLocalpartDomainFromUserid extracts localpart & domain of server from userID
// Returns JSON error in case of invalid username.
func GetLocalpartDomainFromUserid(userID string) (
	string, gomatrixserverlib.ServerName, *util.JSONResponse) {
	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return localpart, domain, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Invalid username"),
		}
	}

	return localpart, domain, nil
}

// Login implements GET and POST /login
func Login(
	req *http.Request, accountDB *accounts.Database, deviceDB *devices.Database,
	cfg config.Dendrite,
) util.JSONResponse {
	if req.Method == http.MethodGet {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: defaultPasswordLogin(),
		}
	} else if req.Method == http.MethodPost {
		var r authtypes.LoginRequest

		resErr := httputil.UnmarshalJSONRequest(req, &r)
		if resErr != nil {
			return *resErr
		}

		util.GetLogger(req.Context()).WithField("user", r.User).Info("Processing login request")

		switch r.Type {
		case authtypes.LoginTypePassword:
			return handlePasswordLogin(r, accountDB, deviceDB, req, cfg)
		case authtypes.LoginTypeToken:
			return handleTokenLogin(r, deviceDB, req, cfg)
		default:
			return util.JSONResponse{
				Code: http.StatusNotImplemented,
				JSON: jsonerror.Unknown("Unknown login type"),
			}
		}
	}
	return util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

func handlePasswordLogin(
	r authtypes.LoginRequest, accountDB *accounts.Database, deviceDB *devices.Database,
	req *http.Request, cfg config.Dendrite) util.JSONResponse {

	if r.User == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("'user' must be supplied."),
		}
	}

	// r.User can either be a user ID or just the localpart... or other things maybe.
	localpart := r.User

	if strings.HasPrefix(r.User, "@") {
		var domain gomatrixserverlib.ServerName
		var err *util.JSONResponse

		localpart, domain, err = GetLocalpartDomainFromUserid(r.User)
		if err != nil {
			return *err
		}

		if domain != cfg.Matrix.ServerName {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername("User ID not ours"),
			}
		}
	}

	_, err := accountDB.GetAccountByPassword(req.Context(), localpart, r.Password)

	if err != nil {
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("username or password was incorrect, or the account does not exist"),
		}
	}

	return completeLogin(r, localpart, deviceDB, req, cfg)
}

func handleTokenLogin(
	r authtypes.LoginRequest, deviceDB *devices.Database,
	req *http.Request, cfg config.Dendrite) util.JSONResponse {
	if r.Token == "" {
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.MissingToken("Missing login token"),
		}
	}

	// UserID may be left empty for token based login
	userID := r.User

	if userID == "" {
		var err error
		userID, err = tokens.GetUserFromToken(r.Token)

		if err != nil {
			return util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.UnknownToken(err.Error()),
			}
		}
	}

	// r.User can either be a user ID or just the localpart... or other things maybe.
	localpart := userID

	if strings.HasPrefix(userID, "@") {
		var domain gomatrixserverlib.ServerName
		var resErr *util.JSONResponse

		localpart, domain, resErr = GetLocalpartDomainFromUserid(r.User)
		if resErr != nil {
			return *resErr
		}

		if domain != cfg.Matrix.ServerName {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername("User ID not ours"),
			}
		}
	}

	tokenOptions := tokens.TokenOptions{
		// ServerMacaroonSecret should be a []byte
		ServerMacaroonSecret: []byte(cfg.Matrix.PrivateKey),
		ServerName:           string(cfg.Matrix.ServerName),
		UserID:               userID,
	}

	err := tokens.ValidateToken(tokenOptions, r.Token)

	if err != nil {
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.UnknownToken(err.Error()),
		}
	}

	return completeLogin(r, localpart, deviceDB, req, cfg)
}

// completeLogin completes the login process after the client request has been
// authenticated by one of the supported methods.
func completeLogin(r authtypes.LoginRequest, localpart string,
	deviceDB *devices.Database, req *http.Request, cfg config.Dendrite,
) util.JSONResponse {
	accessToken, err := auth.GenerateAccessToken()
	if err != nil {
		httputil.LogThenError(req, err)
	}

	// TODO: Use the device ID in the request
	dev, err := deviceDB.CreateDevice(
		req.Context(), localpart, nil, accessToken, r.InitialDisplayName,
	)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: authtypes.LoginResponse{
			UserID:      dev.UserID,
			AccessToken: dev.AccessToken,
			HomeServer:  cfg.Matrix.ServerName,
			DeviceID:    dev.ID,
		},
	}
}
