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
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/common/config"
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
	// Check to see if this is an application service user
	dev, err := verifyApplicationServiceUser(req, data)
	if err != nil {
		return nil, err
	} else if dev != nil {
		return dev, nil
	}

	// Try to find local user from device database
	dev, err = verifyAccessToken(req, data.DeviceDB)
	if err == nil {
		return dev, nil
	}

	return nil, &util.JSONResponse{
		Code: http.StatusUnauthorized,
		JSON: jsonerror.UnknownToken("Unrecognized access token"),
	}
}

// verifyApplicationServiceUser attempts to retrieve a userID given a request
// originating from an application service
func verifyApplicationServiceUser(
	req *http.Request, data Data,
) (*authtypes.Device, *util.JSONResponse) {
	// Try to find the Application Service user
	token, err := extractApplicationServiceAccessToken(req)
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

	userID := req.URL.Query().Get("user_id")
	if appService != nil {
		// Create a dummy device for AS user
		dev := authtypes.Device{
			// Use AS dummy device ID
			ID: types.AppServiceDeviceID,
			// AS dummy device has AS's token.
			AccessToken: token,
		}

		// Check for user masquerading
		if userID != "" {
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
		} else {
			// AS is not masquerading as any user, so use AS's sender_localpart
			dev.UserID = appService.SenderLocalpart
		}
		return &dev, nil
	}

	// Application service was not found with this token
	return nil, nil
}

// verifyAccessToken verifies that an access token was supplied in the given HTTP request
// and returns the device it corresponds to. Returns resErr (an error response which can be
// sent to the client) if the token is invalid or there was a problem querying the database.
func verifyAccessToken(req *http.Request, deviceDB DeviceDatabase) (device *authtypes.Device, resErr *util.JSONResponse) {
	token, err := extractUserAccessToken(req)
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
			jsonErr := httputil.LogThenError(req, err)
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

// extractApplicationServiceAccessToken retreives an access token from a request
// originating from an application service
func extractApplicationServiceAccessToken(req *http.Request) (string, error) {
	authBearer := req.Header.Get("Authorization")
	token := req.URL.Query().Get("access_token")
	if authBearer != "" && token != "" {
		return "", fmt.Errorf("mixing Authorization headers and access_token query parameters")
	}
	if token != "" {
		return token, nil
	}

	return "", fmt.Errorf("missing access token")
}

// extractUserAccessToken retreives an access token from a request originating
// from a non-application service
func extractUserAccessToken(req *http.Request) (string, error) {
	authBearer := req.Header.Get("Authorization")

	if authBearer != "" {
		parts := strings.SplitN(authBearer, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return "", fmt.Errorf("invalid Authorization header")
		}
		return parts[1], nil
	}

	return "", fmt.Errorf("missing access token")
}
