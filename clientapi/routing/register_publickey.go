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

func newPublicKeyAuthSession(request *registerRequest) {
	// Public key auth does not use password. But the registration flow
	// requires setting a password in order to create the account.
	// Create a random password to satisfy the requirement.
	request.Password = util.RandomString(sessionIDLength)
}

func handlePublicKeyRegistration(
	cfg *config.ClientAPI,
	reqBytes []byte,
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
		return false, "", nil
	}

	isCompleted, jerr := authHandler.ValidateLoginResponse()
	if jerr != nil {
		return false, "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jerr,
		}
	}

	return isCompleted, authtypes.LoginType(authHandler.GetType()), nil
}
