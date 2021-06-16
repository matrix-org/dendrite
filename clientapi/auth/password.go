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
	"fmt"
	"net/http"

	"github.com/go-ldap/ldap/v3"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type GetAccountByPassword func(ctx context.Context, localpart, password string) (*api.Account, error)

type GetAccountByLocalpart func(ctx context.Context, localpart string) (*api.Account, error)

type PasswordRequest struct {
	Login
	Password string `json:"password"`
}

// LoginTypePassword implements https://matrix.org/docs/spec/client_server/r0.6.1#password-based
type LoginTypePassword struct {
	GetAccountByPassword  GetAccountByPassword
	GetAccountByLocalpart GetAccountByLocalpart
	Config                *config.ClientAPI
	UserAPI               api.UserInternalAPI
}

func (t *LoginTypePassword) Name() string {
	return "m.login.password"
}

func (t *LoginTypePassword) Request() interface{} {
	return &PasswordRequest{}
}

func (t *LoginTypePassword) Login(ctx context.Context, req interface{}) (*Login, *util.JSONResponse) {
	r := req.(*PasswordRequest)
	username := r.Username()
	if username == "" {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.BadJSON("'user' must be supplied."),
		}
	}
	localpart, err := userutil.ParseUsernameParam(username, &t.Config.Matrix.ServerName)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}
	if len(t.Config.LDAP.Host) > 0 {
		addr := ""
		if t.Config.LDAP.TLS {
			addr = "ldaps://" + t.Config.LDAP.Host + ":" + t.Config.LDAP.Port
		} else {
			addr = "ldap://" + t.Config.LDAP.Host + ":" + t.Config.LDAP.Port
		}

		var conn *ldap.Conn
		conn, err = ldap.DialURL(addr)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}
		defer conn.Close()

		e1 := conn.Bind(t.Config.LDAP.BindDN, t.Config.LDAP.BindPSWD)
		if e1 != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}
		filter := fmt.Sprintf("(&%s(%s=%s))", t.Config.LDAP.Filter, "uid", localpart)
		searchRequest := ldap.NewSearchRequest(t.Config.LDAP.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, filter, []string{"uid"}, nil)
		var sr *ldap.SearchResult
		sr, err = conn.Search(searchRequest)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}
		if len(sr.Entries) > 1 {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.BadJSON("'user' must be duplicated."),
			}
		}
		if len(sr.Entries) == 0 {
			_, err = t.GetAccountByPassword(ctx, localpart, r.Password)
			if err != nil {
				return nil, &util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("username or password was incorrect, or the account does not exist"),
				}
			}
			return &r.Login, nil
		}

		userDN := sr.Entries[0].DN
		err = conn.Bind(userDN, r.Password)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}

		_, err = t.GetAccountByLocalpart(ctx, localpart)
		if err != nil {
			if err == sql.ErrNoRows {
				var accRes api.PerformAccountCreationResponse
				err = t.UserAPI.PerformAccountCreation(ctx, &api.PerformAccountCreationRequest{
					AppServiceID: "",
					Localpart:    localpart,
					Password:     r.Password,
					AccountType:  api.AccountTypeUser,
					OnConflict:   api.ConflictAbort,
				}, &accRes)
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

				return &r.Login, nil
			}

			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.InvalidUsername(err.Error()),
			}
		}
		return &r.Login, nil
	}
	_, err = t.GetAccountByPassword(ctx, localpart, r.Password)
	if err != nil {
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("username or password was incorrect, or the account does not exist"),
		}
	}
	return &r.Login, nil
}
