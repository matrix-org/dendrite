// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package auth

import (
	"context"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/util"
)

// LoginTypeApplicationService describes how to authenticate as an
// application service
type LoginTypeApplicationService struct {
	Config *config.ClientAPI
	Token  string
}

// Name implements Type
func (t *LoginTypeApplicationService) Name() string {
	return authtypes.LoginTypeApplicationService
}

// LoginFromJSON implements Type
func (t *LoginTypeApplicationService) LoginFromJSON(
	ctx context.Context, reqBytes []byte,
) (*Login, LoginCleanupFunc, *util.JSONResponse) {
	var r Login
	if err := httputil.UnmarshalJSON(reqBytes, &r); err != nil {
		return nil, nil, err
	}

	_, err := internal.ValidateApplicationServiceRequest(t.Config, r.Identifier.User, t.Token)
	if err != nil {
		return nil, nil, err
	}

	cleanup := func(ctx context.Context, j *util.JSONResponse) {}
	return &r, cleanup, nil
}
