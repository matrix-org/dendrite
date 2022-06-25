// Copyright 2022 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
)

func handlePublicKeyRegistration(
	cfg *config.ClientAPI,
	reqBytes []byte,
	r *registerRequest,
	userAPI userapi.ClientUserAPI,
) (bool, authtypes.LoginType, *util.JSONResponse) {
	if !cfg.PublicKeyAuthentication.Enabled() {
		return false, "", &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("public key account registration is disabled"),
		}
	}

	var authHandler auth.LoginPublicKeyHandler
	authType := gjson.GetBytes(reqBytes, "auth.public_key_response.type").String()

	switch authType {
	case authtypes.LoginTypePublicKeyEthereum:
		authBytes := gjson.GetBytes(reqBytes, "auth.public_key_response")
		pkEthHandler, err := auth.CreatePublicKeyEthereumHandler(
			[]byte(authBytes.Raw),
			userAPI,
			cfg,
		)
		if err != nil {
			return false, "", &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: err,
			}
		}
		authHandler = pkEthHandler
	default:
		// No response. Client is asking for a new registration session
		return false, authtypes.LoginStagePublicKeyNewSession, nil
	}

	if _, ok := sessions.sessions[authHandler.GetSession()]; !ok {
		return false, "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.Unknown("the session ID is missing or unknown."),
		}
	}

	isValidUserId := authHandler.IsValidUserIdForRegistration(r.Username)
	if !isValidUserId {
		return false, "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(r.Username),
		}
	}

	isValidated, jerr := authHandler.ValidateLoginResponse()
	if jerr != nil {
		return false, "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jerr,
		}
	}

	// Registration flow requires a password to
	// create a user account. Create a random one
	// to satisfy the requirement. This is not used
	// for public key cryptography.
	createPassword(r)

	return isValidated, authtypes.LoginType(authHandler.GetType()), nil
}

func createPassword(request *registerRequest) {
	// Public key auth does not use password.
	// Create a random one that is never used.
	// Login validation will be done using public / private
	// key cryptography.
	request.Password = util.RandomString(sessionIDLength)
}
