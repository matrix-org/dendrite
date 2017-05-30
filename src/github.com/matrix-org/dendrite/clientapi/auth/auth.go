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
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// UnknownDeviceID is the default device id if one is not specified.
// This deviates from Synapse which generates a new device ID if one is not specified.
// It's preferable to not amass a huge list of valid access tokens for an account,
// so limiting it to 1 unknown device for now limits the number of valid tokens.
// Clients should be giving us device IDs.
var UnknownDeviceID = "unknown-device"

// OWASP recommends at least 128 bits of entropy for tokens: https://www.owasp.org/index.php/Insufficient_Session-ID_Length
// 32 bytes => 256 bits
var tokenByteLength = 32

// VerifyAccessToken verifies that an access token was supplied in the given HTTP request
// and returns the device it corresponds to. Returns resErr (an error response which can be
// sent to the client) if the token is invalid or there was a problem querying the database.
func VerifyAccessToken(req *http.Request, deviceDB *devices.Database) (device *authtypes.Device, resErr *util.JSONResponse) {
	token, err := extractAccessToken(req)
	if err != nil {
		resErr = &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.MissingToken(err.Error()),
		}
		return
	}
	device, err = deviceDB.GetDeviceByAccessToken(token)
	if err != nil {
		if err == sql.ErrNoRows {
			resErr = &util.JSONResponse{
				Code: 403,
				JSON: jsonerror.Forbidden("Invalid access token"),
			}
		} else {
			resErr = &util.JSONResponse{
				Code: 500,
				JSON: jsonerror.Unknown("Failed to check access token"),
			}
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

// extractAccessToken from a request, or return an error detailing what went wrong. The
// error message MUST be human-readable and comprehensible to the client.
func extractAccessToken(req *http.Request) (string, error) {
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
