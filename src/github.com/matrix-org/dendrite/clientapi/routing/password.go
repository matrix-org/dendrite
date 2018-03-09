// Copyright 2017 Vector Creations Ltd
// Copyright 2018 New Vector Ltd
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
	"github.com/matrix-org/dendrite/common/config"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// changePasswordRequest represents the submitted request from client side.
// It can be broken down into 2 sections: the userInteractiveFlow parameters and
// password change parameters. the 1st set is for calling UIAA handler while
// change password parameters vary depending on the request, and will need to remembered across
// sessions. If no parameters are supplied, the server should use the parameters previously
// remembered. If ANY parameters are supplied, the server should REPLACE all knowledge of
// previous parameters with the ones supplied. This mean you cannot "build up" request params.

// a generic flowRequest type for any auth call to UIAA handler to be included in UIAA.go Todo referenced in (#413)
type userInteractiveFlowRequest struct {
	// parameters
	Password string `json:"password"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`

	// user-interactive auth params
	Auth authDict `json:"auth"`

	// Application Services place Type in the root of their registration
	// request, whereas clients place it in the authDict struct.
	Type authtypes.LoginType `json:"type"`
}

type changePasswordRequest struct {
	// user-interactive auth params (necessary to include to make call to UIAA handler)
	userInteractiveFlowRequest

	// password change parameters
	NewPassword string `json:"new_password"`
}

// ChangePassword implements a /password request.
func ChangePassword(
	req *http.Request,
	accountDB *accounts.Database,
	cfg *config.Dendrite,
) util.JSONResponse {

	// Todo username and password aren't in request as per spec. Use whoami referenced in #411
	var r changePasswordRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	// Retrieve or generate the sessionID
	sessionID := r.Auth.Session
	if sessionID == "" {
		// Generate a new, random session ID
		sessionID = util.RandomString(sessionIDLength)
	}

	// If no auth type is specified by the client, send back the list of available flows
	if r.Auth.Type == "" {
		return util.JSONResponse{
			Code: 401,
			// Todo replace by password.Flows when available
			JSON: newUserInteractiveResponse(sessionID,
				cfg.Derived.Registration.Flows, cfg.Derived.Registration.Params),
		}
	}

	// Squash username to all lowercase letters
	r.Username = strings.ToLower(r.Username)

	// validate new password
	if resErr = validatePassword(r.NewPassword); resErr != nil {
		return *resErr
	}

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"username":   r.Username,
		"auth.type":  r.Auth.Type,
		"session_id": r.Auth.Session,
	}).Info("Processing password request")

	// Todo go through User Interactive Auth uncomment references in #413  and
	// authflows defined in cfg.Derived like that for registration

	// Todo appservice ID is to be returned by UIAA handler use that.
	token, err := auth.ExtractAccessToken(req)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	appServiceID := ""
	for _, as := range cfg.Derived.ApplicationServices {
		if as.ASToken == token {
			appServiceID = as.ID
			break
		}
	}
	// return the final update function after auth flow
	return completeUpdate(req.Context(), accountDB, r.Username, r.NewPassword, appServiceID)
}

func completeUpdate(
	ctx context.Context,
	accountDB *accounts.Database,
	username, password, appserviceID string,
) util.JSONResponse {

	// Blank passwords are only allowed by registered application services
	if password == "" && appserviceID == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("missing password"),
		}
	}

	err := accountDB.UpdatePassword(ctx, password, username)

	if err != nil {
		return util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("failed to update password: " + err.Error()),
		}
	}

	return util.JSONResponse{
		Code: 200,
	}
}
