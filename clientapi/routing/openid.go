// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
			JSON: spec.Forbidden("Cannot request tokens for other users"),
		}
	}

	request := api.PerformOpenIDTokenCreationRequest{
		UserID: userID, // this is the user ID from the incoming path
	}
	response := api.PerformOpenIDTokenCreationResponse{}

	err := userAPI.PerformOpenIDTokenCreation(req.Context(), &request, &response)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("userAPI.CreateOpenIDToken failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
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
