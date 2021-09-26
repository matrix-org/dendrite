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
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// OWASP recommends at least 128 bits of entropy for tokens: https://www.owasp.org/index.php/Insufficient_Session-ID_Length
// 32 bytes => 256 bits
var tokenByteLength = 32

// DeviceDatabase represents a device database.
type DeviceDatabase interface {
	// Look up the device matching the given access token.
	GetDeviceByAccessToken(ctx context.Context, token string) (*api.Device, error)
}

// AccountDatabase represents an account database.
type AccountDatabase interface {
	// Look up the account matching the given localpart.
	GetAccountByLocalpart(ctx context.Context, localpart string) (*api.Account, error)
	GetAccountByPassword(ctx context.Context, localpart, password string) (*api.Account, error)
	GetLocalpartForThreePID(ctx context.Context, address, medium string) (string, error)
}

// VerifyUserFromRequest authenticates the HTTP request,
// on success returns Device of the requester.
// Finds local user or an application service user.
// Note: For an AS user, AS dummy device is returned.
// On failure returns an JSON error response which can be sent to the client.
func VerifyUserFromRequest(
	req *http.Request, userAPI api.QueryAcccessTokenAPI,
) (*api.Device, *util.JSONResponse) {
	// Try to find the Application Service user
	token, err := ExtractAccessToken(req)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.MissingToken(err.Error()),
		}
	}
	var res api.QueryAccessTokenResponse
	err = userAPI.QueryAccessToken(req.Context(), &api.QueryAccessTokenRequest{
		AccessToken:      token,
		AppServiceUserID: req.URL.Query().Get("user_id"),
	}, &res)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("userAPI.QueryAccessToken failed")
		jsonErr := jsonerror.InternalServerError()
		return nil, &jsonErr
	}
	if res.Err != "" {
		if strings.HasPrefix(strings.ToLower(res.Err), "forbidden:") { // TODO: use actual error and no string comparison
			return nil, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden(res.Err),
			}
		}
	}
	if res.Device == nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.UnknownToken("Unknown token"),
		}
	}
	return res.Device, nil
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
