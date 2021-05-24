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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/util"
)

type GetAccountByPassword func(ctx context.Context, localpart, password string) (*api.Account, error)

type PasswordRequest struct {
	Login
	Address  string `json:"address"`
	Password string `json:"password"`
	Medium   string `json:"medium"`
}

// LoginTypePassword implements https://matrix.org/docs/spec/client_server/r0.6.1#password-based
type LoginTypePassword struct {
	GetAccountByPassword GetAccountByPassword
	Config               *config.ClientAPI
	AccountDB            accounts.Database
}

func (t *LoginTypePassword) Name() string {
	return "m.login.password"
}

func (t *LoginTypePassword) Request() interface{} {
	return &PasswordRequest{}
}

func (t *LoginTypePassword) Login(ctx context.Context, req interface{}) (*Login, *util.JSONResponse) {
	r := req.(*PasswordRequest)
	var username string
	var localpart string
	var err error
	username = r.Username()
	if username != "" {
		localpart, err = userutil.ParseUsernameParam(username, &t.Config.Matrix.ServerName)
	} else {
		if r.Medium == "email" {
			if r.Address != "" {
				localpart, err = t.AccountDB.GetLocalpartForThreePID(ctx, r.Address, "email")
				if localpart == "" {
					return nil, &util.JSONResponse{
						Code: http.StatusForbidden,
						JSON: jsonerror.Forbidden("email or password was incorrect, or the account does not exist"),
					}
				}
				r.Login.User = localpart
			} else {
				return nil, &util.JSONResponse{
					Code: http.StatusUnauthorized,
					JSON: jsonerror.BadJSON("'address' must be supplied."),
				}
			}
		} else {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.BadJSON("'user' must be supplied."),
			}
		}
	}
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}
	_, err = t.GetAccountByPassword(ctx, localpart, r.Password)
	if err != nil {
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		return nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The username or password was incorrect or the account does not exist."),
		}
	}
	return &r.Login, nil
}
