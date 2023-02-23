// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package auth

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/matrix-org/gomatrixserverlib"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type PasswordRequest struct {
	Login
	Password string `json:"password"`
}

// LoginTypePassword implements https://matrix.org/docs/spec/client_server/r0.6.1#password-based
type LoginTypePassword struct {
	Config  *config.ClientAPI
	UserAPI api.UserLoginAPI
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

func (t *LoginTypePassword) Login(ctx context.Context, request *PasswordRequest) (*Login, *util.JSONResponse) {
	fullUsername := request.Username()
	if fullUsername == "" {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.BadJSON("A username must be supplied."),
		}
	}
	if len(request.Password) == 0 {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.BadJSON("A password must be supplied."),
		}
	}
	username, domain, err := userutil.ParseUsernameParam(fullUsername, t.Config.Matrix)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}
	if !t.Config.Matrix.IsLocalServerName(domain) {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername("The server name is not known."),
		}
	}

	var account *api.Account
	if t.Config.Ldap.Enabled {
		ldapAuthenticator := NewLdapAuthenticator(t.Config.Ldap)
		isAdmin, err := ldapAuthenticator.Authenticate(username, request.Password)
		if err != nil {
			return nil, err
		}
		acc, err := t.getOrCreateAccount(ctx, username, domain, isAdmin)
		if err != nil {
			return nil, err
		}
		account = acc
	} else {
		acc, err := t.authenticateDb(ctx, username, domain, request.Password)
		if err != nil {
			return nil, err
		}
		account = acc
	}

	// Set the user, so login.Username() can do the right thing
	request.Identifier.User = account.UserID
	request.User = account.UserID
	return &request.Login, nil
}

func (t *LoginTypePassword) authenticateDb(ctx context.Context, username string, domain gomatrixserverlib.ServerName, password string) (*api.Account, *util.JSONResponse) {
	res := &api.QueryAccountByPasswordResponse{}
	err := t.UserAPI.QueryAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
		Localpart:         strings.ToLower(username),
		ServerName:        domain,
		PlaintextPassword: password,
	}, res)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("Unable to fetch account by password."),
		}
	}

	if !res.Exists {
		err = t.UserAPI.QueryAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
			Localpart:         username,
			ServerName:        domain,
			PlaintextPassword: password,
		}, res)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: jsonerror.Unknown("Unable to fetch account by password."),
			}
		}
		if !res.Exists {
			return nil, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden("The username or password was incorrect or the account does not exist."),
			}
		}
	}
	return res.Account, nil
}

func (t *LoginTypePassword) getOrCreateAccount(ctx context.Context, username string, domain gomatrixserverlib.ServerName, admin bool) (*api.Account, *util.JSONResponse) {
	var existing api.QueryAccountByLocalpartResponse
	err := t.UserAPI.QueryAccountByLocalpart(ctx, &api.QueryAccountByLocalpartRequest{
		Localpart:  username,
		ServerName: domain,
	}, &existing)

	if err == nil {
		return existing.Account, nil
	}
	if err != sql.ErrNoRows {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}

	accountType := api.AccountTypeUser
	if admin {
		accountType = api.AccountTypeAdmin
	}
	var created api.PerformAccountCreationResponse
	err = t.UserAPI.PerformAccountCreation(ctx, &api.PerformAccountCreationRequest{
		AppServiceID: "ldap",
		Localpart:    username,
		Password:     uuid.New().String(),
		AccountType:  accountType,
		OnConflict:   api.ConflictAbort,
	}, &created)

	if err != nil {
		if _, ok := err.(*api.ErrorConflict); ok {
			return nil, &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.UserInUse("Desired user ID is already taken."),
			}
		}
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create account: " + err.Error()),
		}
	}
	return created.Account, nil
}
