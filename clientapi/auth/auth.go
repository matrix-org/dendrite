// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

// Package auth implements authentication checks and storage.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
			JSON: spec.MissingToken(err.Error()),
		}
	}
	var res api.QueryAccessTokenResponse
	err = userAPI.QueryAccessToken(req.Context(), &api.QueryAccessTokenRequest{
		AccessToken:      token,
		AppServiceUserID: req.URL.Query().Get("user_id"),
	}, &res)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("userAPI.QueryAccessToken failed")
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if res.Err != "" {
		if strings.HasPrefix(strings.ToLower(res.Err), "forbidden:") { // TODO: use actual error and no string comparison
			return nil, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden(res.Err),
			}
		}
	}
	if res.Device == nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.UnknownToken("Unknown token"),
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
