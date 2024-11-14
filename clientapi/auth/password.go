// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/clientapi/userutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type GetAccountByPassword func(ctx context.Context, req *api.QueryAccountByPasswordRequest, res *api.QueryAccountByPasswordResponse) error

type PasswordRequest struct {
	Login
	Password string `json:"password"`
}

// LoginTypePassword implements https://matrix.org/docs/spec/client_server/r0.6.1#password-based
type LoginTypePassword struct {
	GetAccountByPassword GetAccountByPassword
	Config               *config.ClientAPI
}

func (t *LoginTypePassword) Name() string {
	return authtypes.LoginTypePassword
}

func (t *LoginTypePassword) LoginFromJSON(ctx context.Context, reqBytes []byte) (*Login, LoginCleanupFunc, *util.JSONResponse) {
	var r PasswordRequest
	if err := httputil.UnmarshalJSON(reqBytes, &r); err != nil {
		return nil, nil, err
	}

	login, err := t.Login(ctx, &r)
	if err != nil {
		return nil, nil, err
	}

	return login, func(context.Context, *util.JSONResponse) {}, nil
}

func (t *LoginTypePassword) Login(ctx context.Context, req interface{}) (*Login, *util.JSONResponse) {
	r := req.(*PasswordRequest)
	username := r.Username()
	if username == "" {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.BadJSON("A username must be supplied."),
		}
	}
	if len(r.Password) == 0 {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.BadJSON("A password must be supplied."),
		}
	}
	localpart, domain, err := userutil.ParseUsernameParam(username, t.Config.Matrix)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.InvalidUsername(err.Error()),
		}
	}
	if !t.Config.Matrix.IsLocalServerName(domain) {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.InvalidUsername("The server name is not known."),
		}
	}
	// Squash username to all lowercase letters
	res := &api.QueryAccountByPasswordResponse{}
	err = t.GetAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
		Localpart:         strings.ToLower(localpart),
		ServerName:        domain,
		PlaintextPassword: r.Password,
	}, res)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("Unable to fetch account by password."),
		}
	}

	// If we couldn't find the user by the lower cased localpart, try the provided
	// localpart as is.
	if !res.Exists {
		err = t.GetAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
			Localpart:         localpart,
			ServerName:        domain,
			PlaintextPassword: r.Password,
		}, res)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown("Unable to fetch account by password."),
			}
		}
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		if !res.Exists {
			return nil, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden("The username or password was incorrect or the account does not exist."),
			}
		}
	}
	// Set the user, so login.Username() can do the right thing
	r.Identifier.User = res.Account.UserID
	r.User = res.Account.UserID
	return &r.Login, nil
}
