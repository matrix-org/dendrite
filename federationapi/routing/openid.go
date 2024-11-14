// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"
	"time"

	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type openIDUserInfoResponse struct {
	Sub string `json:"sub"`
}

// GetOpenIDUserInfo implements GET /_matrix/federation/v1/openid/userinfo
func GetOpenIDUserInfo(
	httpReq *http.Request,
	userAPI userapi.FederationUserAPI,
) util.JSONResponse {
	token := httpReq.URL.Query().Get("access_token")
	if len(token) == 0 {
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.MissingParam("access_token is missing"),
		}
	}

	req := userapi.QueryOpenIDTokenRequest{
		Token: token,
	}

	var openIDTokenAttrResponse userapi.QueryOpenIDTokenResponse
	err := userAPI.QueryOpenIDToken(httpReq.Context(), &req, &openIDTokenAttrResponse)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("userAPI.QueryOpenIDToken failed")
	}

	var res interface{} = openIDUserInfoResponse{Sub: openIDTokenAttrResponse.Sub}
	code := http.StatusOK
	nowMS := time.Now().UnixNano() / int64(time.Millisecond)
	if openIDTokenAttrResponse.Sub == "" || nowMS > openIDTokenAttrResponse.ExpiresAtMS {
		code = http.StatusUnauthorized
		res = spec.UnknownToken("Access Token unknown or expired")
	}

	return util.JSONResponse{
		Code: code,
		JSON: res,
	}
}
