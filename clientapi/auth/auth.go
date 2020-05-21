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

// Package auth implements authentication checks and storage.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/appservice/types"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/util"
)

// OWASP recommends at least 128 bits of entropy for tokens: https://www.owasp.org/index.php/Insufficient_Session-ID_Length
// 32 bytes => 256 bits
var tokenByteLength = 32

// DeviceDatabase represents a device database.
type DeviceDatabase interface {
	// Look up the device matching the given access token.
	GetDeviceByAccessToken(ctx context.Context, token string) (*authtypes.Device, error)
}

// AccountDatabase represents an account database.
type AccountDatabase interface {
	// Look up the account matching the given localpart.
	GetAccountByLocalpart(ctx context.Context, localpart string) (*authtypes.Account, error)
}

// Data contains information required to authenticate a request.
type Data struct {
	AccountDB AccountDatabase
	DeviceDB  DeviceDatabase
	// AppServices is the list of all registered AS
	AppServices []config.ApplicationService
}

// VerifyUserFromRequest authenticates the HTTP request,
// on success returns Device of the requester.
// Finds local user or an application service user.
// Note: For an AS user, AS dummy device is returned.
// On failure returns an JSON error response which can be sent to the client.
func VerifyUserFromRequest(
	req *http.Request, data Data,
) (*authtypes.Device, *util.JSONResponse) {
	// Try to find the Application Service user
	token, err := ExtractAccessToken(req)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.MissingToken(err.Error()),
		}
	}

	// Search for app service with given access_token
	var appService *config.ApplicationService
	for _, as := range data.AppServices {
		if as.ASToken == token {
			appService = &as
			break
		}
	}

	if appService != nil {
		// Create a dummy device for AS user
		dev := authtypes.Device{
			// Use AS dummy device ID
			ID: types.AppServiceDeviceID,
			// AS dummy device has AS's token.
			AccessToken: token,
		}

		userID := req.URL.Query().Get("user_id")
		localpart, err := userutil.ParseUsernameParam(userID, nil)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}

		if localpart != "" { // AS is masquerading as another user
			// Verify that the user is registered
			account, err := data.AccountDB.GetAccountByLocalpart(req.Context(), localpart)
			// Verify that account exists & appServiceID matches
			if err == nil && account.AppServiceID == appService.ID {
				// Set the userID of dummy device
				dev.UserID = userID
				return &dev, nil
			}

			return nil, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden("Application service has not registered this user"),
			}
		}

		// AS is not masquerading as any user, so use AS's sender_localpart
		dev.UserID = appService.SenderLocalpart
		return &dev, nil
	}

	// Try to find local user from device database
	dev, devErr := verifyAccessToken(req, data.DeviceDB)
	if devErr == nil {
		return dev, verifyUserParameters(req)
	}

	return nil, &util.JSONResponse{
		Code: http.StatusUnauthorized,
		JSON: jsonerror.UnknownToken("Unrecognized access token"), // nolint: misspell
	}
}

// verifyUserParameters ensures that a request coming from a regular user is not
// using any query parameters reserved for an application service
func verifyUserParameters(req *http.Request) *util.JSONResponse {
	if req.URL.Query().Get("ts") != "" {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("parameter 'ts' not allowed without valid parameter 'access_token'"),
		}
	}
	return nil
}

// verifyAccessToken verifies that an access token was supplied in the given HTTP request
// and returns the device it corresponds to. Returns resErr (an error response which can be
// sent to the client) if the token is invalid or there was a problem querying the database.
func verifyAccessToken(req *http.Request, deviceDB DeviceDatabase) (device *authtypes.Device, resErr *util.JSONResponse) {
	token, err := ExtractAccessToken(req)
	if err != nil {
		resErr = &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.MissingToken(err.Error()),
		}
		return
	}
	device, err = deviceDB.GetDeviceByAccessToken(req.Context(), token)
	if err != nil {
		if err == sql.ErrNoRows {
			resErr = &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.UnknownToken("Unknown token"),
			}
		} else {
			util.GetLogger(req.Context()).WithError(err).Error("deviceDB.GetDeviceByAccessToken failed")
			jsonErr := jsonerror.InternalServerError()
			resErr = &jsonErr
		}
	}
	return
}

// GenerateAccessToken creates a new access token. Returns an error if failed to generate
// random bytes.
func GenerateAccessToken() (string, error) {
	b := make([]byte, tokenByteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// url-safe no padding
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ExtractAccessToken from a request, or return an error detailing what went wrong. The
// error message MUST be human-readable and comprehensible to the client.
func ExtractAccessToken(req *http.Request) (string, error) {
	// cf https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/api/auth.py#L631
	authBearer := req.Header.Get("Authorization")
	queryToken := req.URL.Query().Get("access_token")
	if authBearer != "" && queryToken != "" {
		return "", fmt.Errorf("mixing Authorization headers and access_token query parameters")
	}

	if queryToken != "" {
		return queryToken, nil
	}

	if authBearer != "" {
		parts := strings.SplitN(authBearer, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return "", fmt.Errorf("invalid Authorization header")
		}
		return parts[1], nil
	}

	return "", fmt.Errorf("missing access token")
}
