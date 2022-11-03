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
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/ratelimit"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

type GetAccountByPassword func(ctx context.Context, req *api.QueryAccountByPasswordRequest, res *api.QueryAccountByPasswordResponse) error

type PasswordRequest struct {
	Login
	Password string `json:"password"`
	Address  string `json:"address"`
	Medium   string `json:"medium"`
}

const email = "email"

// LoginTypePassword implements https://matrix.org/docs/spec/client_server/r0.6.1#password-based
type LoginTypePassword struct {
	UserApi       api.ClientUserAPI
	Config        *config.ClientAPI
	Rt            *ratelimit.RtFailedLogin
	InhibitDevice bool
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
	login.InhibitDevice = t.InhibitDevice

	return login, func(context.Context, *util.JSONResponse) {}, nil
}

func (t *LoginTypePassword) Login(ctx context.Context, req interface{}) (*Login, *util.JSONResponse) {
	r := req.(*PasswordRequest)
	if r.Identifier.Address != "" {
		r.Address = r.Identifier.Address
	}
	if r.Identifier.Medium != "" {
		r.Medium = r.Identifier.Medium
	}
	var username string
	if r.Medium == email && r.Address != "" {
		r.Address = strings.ToLower(r.Address)
		res := api.QueryLocalpartForThreePIDResponse{}
		err := t.UserApi.QueryLocalpartForThreePID(ctx, &api.QueryLocalpartForThreePIDRequest{
			ThreePID: r.Address,
			Medium:   email,
		}, &res)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("userApi.QueryLocalpartForThreePID failed")
			resp := jsonerror.InternalServerError()
			return nil, &resp
		}
		username = res.Localpart
		if username == "" {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: jsonerror.Forbidden("Invalid username or password"),
			}
		}
	} else {
		username = strings.ToLower(r.Username())
	}
	if username == "" {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.BadJSON("A username must be supplied."),
		}
	}
	if len(r.Password) == 0 {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.BadJSON("A password must be supplied."),
		}
	}
	localpart, _, err := userutil.ParseUsernameParam(username, t.Config.Matrix)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidUsername(err.Error()),
		}
	}
	// Squash username to all lowercase letters
	res := &api.QueryAccountByPasswordResponse{}
	localpart = strings.ToLower(localpart)
	if t.Rt != nil {
		ok, retryIn := t.Rt.CanAct(localpart)
		if !ok {
			return nil, &util.JSONResponse{
				Code: http.StatusTooManyRequests,
				JSON: jsonerror.LimitExceeded("Too Many Requests", retryIn.Milliseconds()),
			}
		}
	}
	err = t.UserApi.QueryAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{Localpart: localpart, PlaintextPassword: r.Password}, res)
	if err != nil {
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("unable to fetch account by password"),
		}
	}

	if !res.Exists {
		err = t.UserApi.QueryAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
			Localpart:         localpart,
			PlaintextPassword: r.Password,
		}, res)
		if err != nil {
			return nil, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: jsonerror.Unknown("unable to fetch account by password"),
			}
		}
		// Technically we could tell them if the user does not exist by checking if err == sql.ErrNoRows
		// but that would leak the existence of the user.
		if !res.Exists {
			if t.Rt != nil {
				t.Rt.Act(localpart)
			}
			return nil, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden("Invalid username or password"),
			}
		}
	}
	r.Login.User = username
	return &r.Login, nil
}
