// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package auth

import (
	"context"
	"net/http"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/setup/config"
	uapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// LoginTypeToken describes how to authenticate with a login token.
type LoginTypeToken struct {
	UserAPI uapi.LoginTokenInternalAPI
	Config  *config.ClientAPI
}

// Name implements Type.
func (t *LoginTypeToken) Name() string {
	return authtypes.LoginTypeToken
}

// LoginFromJSON implements Type. The cleanup function deletes the token from
// the database on success.
func (t *LoginTypeToken) LoginFromJSON(ctx context.Context, reqBytes []byte) (*Login, LoginCleanupFunc, *util.JSONResponse) {
	var r loginTokenRequest
	if err := httputil.UnmarshalJSON(reqBytes, &r); err != nil {
		return nil, nil, err
	}

	var res uapi.QueryLoginTokenResponse
	if err := t.UserAPI.QueryLoginToken(ctx, &uapi.QueryLoginTokenRequest{Token: r.Token}, &res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("UserAPI.QueryLoginToken failed")
		return nil, nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if res.Data == nil {
		return nil, nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("invalid login token"),
		}
	}

	r.Login.Identifier.Type = "m.id.user"
	r.Login.Identifier.User = res.Data.UserID

	cleanup := func(ctx context.Context, authRes *util.JSONResponse) {
		if authRes == nil {
			util.GetLogger(ctx).Error("No JSONResponse provided to LoginTokenType cleanup function")
			return
		}
		if authRes.Code == http.StatusOK {
			var res uapi.PerformLoginTokenDeletionResponse
			if err := t.UserAPI.PerformLoginTokenDeletion(ctx, &uapi.PerformLoginTokenDeletionRequest{Token: r.Token}, &res); err != nil {
				util.GetLogger(ctx).WithError(err).Error("UserAPI.PerformLoginTokenDeletion failed")
			}
		}
	}
	return &r.Login, cleanup, nil
}

// loginTokenRequest struct to hold the possible parameters from an HTTP request.
type loginTokenRequest struct {
	Login
	Token string `json:"token"`
}
