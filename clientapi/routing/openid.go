// Copyright 2021 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type openIDTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	MatrixServerName string `json:"matrix_server_name"`
	ExpiresIn        int64  `json:"expires_in"`
}

// CreateOpenIDToken creates a new OpenID Connect (OIDC) token that a Matrix user
// can supply to an OpenID Relying Party to verify their identity
func CreateOpenIDToken(
	req *http.Request,
	userAPI api.ClientUserAPI,
	device *api.Device,
	userID string,
	cfg *config.ClientAPI,
) util.JSONResponse {
	// does the incoming user ID match the user that the token was issued for?
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot request tokens for other users"),
		}
	}

	request := api.PerformOpenIDTokenCreationRequest{
		UserID: userID, // this is the user ID from the incoming path
	}
	response := api.PerformOpenIDTokenCreationResponse{}

	err := userAPI.PerformOpenIDTokenCreation(req.Context(), &request, &response)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("userAPI.CreateOpenIDToken failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: openIDTokenResponse{
			AccessToken:      response.Token.Token,
			TokenType:        "Bearer",
			MatrixServerName: string(device.UserDomain()),
			ExpiresIn:        response.Token.ExpiresAtMS / 1000, // convert ms to s
		},
	}
}
