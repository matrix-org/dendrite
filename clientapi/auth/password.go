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
	"net/http"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/ratelimit"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

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
	UserLoginAPI  api.UserLoginAPI
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
			return nil, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown(""),
			}
		}
		username = "@" + res.Localpart + ":" + string(t.Config.Matrix.ServerName)
		if username == "" {
			return nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: spec.Forbidden("Invalid username or password"),
			}
		}
	} else {
		username = r.Username()
	}
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
	if t.Rt != nil {
		ok, retryIn := t.Rt.CanAct(localpart)
		if !ok {
			return nil, &util.JSONResponse{
				Code: http.StatusTooManyRequests,
				JSON: spec.LimitExceeded("Too Many Requests", retryIn.Milliseconds()),
			}
		}
	}
	err = t.UserApi.QueryAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
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

	var account *api.Account
	if t.Config.Ldap.Enabled {
		isAdmin, err := t.authenticateLdap(username, r.Password)
		if err != nil {
			return nil, err
		}
		acc, err := t.getOrCreateAccount(ctx, localpart, domain, isAdmin)
		if err != nil {
			return nil, err
		}
		account = acc
	} else {
		acc, err := t.authenticateDb(ctx, localpart, domain, r.Password)
		if err != nil {
			return nil, err
		}
		account = acc
	}

	// Set the user, so login.Username() can do the right thing
	r.Identifier.User = account.UserID
	r.User = account.UserID
	r.Login.User = username
	return &r.Login, nil
}

func (t *LoginTypePassword) authenticateDb(ctx context.Context, localpart string, domain spec.ServerName, password string) (*api.Account, *util.JSONResponse) {
	res := &api.QueryAccountByPasswordResponse{}
	err := t.UserApi.QueryAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
		Localpart:         strings.ToLower(localpart),
		ServerName:        domain,
		PlaintextPassword: password,
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
		err = t.UserLoginAPI.QueryAccountByPassword(ctx, &api.QueryAccountByPasswordRequest{
			Localpart:         localpart,
			ServerName:        domain,
			PlaintextPassword: password,
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
			if t.Rt != nil {
				t.Rt.Act(localpart)
			}
			return nil, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden("The username or password was incorrect or the account does not exist."),
			}
		}
	}
	return res.Account, nil
}

func (t *LoginTypePassword) authenticateLdap(username, password string) (bool, *util.JSONResponse) {
	var conn *ldap.Conn
	conn, err := ldap.DialURL(t.Config.Ldap.Uri)
	if err != nil {
		return false, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("unable to connect to ldap: " + err.Error()),
		}
	}
	// nolint: errcheck
	defer conn.Close()

	if t.Config.Ldap.AdminBindEnabled {
		err = conn.Bind(t.Config.Ldap.AdminBindDn, t.Config.Ldap.AdminBindPassword)
		if err != nil {
			return false, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown("unable to bind to ldap: " + err.Error()),
			}
		}
		filter := strings.ReplaceAll(t.Config.Ldap.SearchFilter, "{username}", username)
		searchRequest := ldap.NewSearchRequest(
			t.Config.Ldap.BaseDn, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			0, 0, false, filter, []string{t.Config.Ldap.SearchAttribute}, nil,
		)
		var result *ldap.SearchResult
		result, err = conn.Search(searchRequest)
		if err != nil {
			return false, &util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown("unable to bind to search ldap: " + err.Error()),
			}
		}
		if len(result.Entries) > 1 {
			return false, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: spec.BadJSON("'user' must be duplicated."),
			}
		}
		if len(result.Entries) < 1 {
			return false, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: spec.BadJSON("'user' not found."),
			}
		}

		userDN := result.Entries[0].DN
		err = conn.Bind(userDN, password)
		if err != nil {
			var localpart string
			localpart, _, err = userutil.ParseUsernameParam(username, t.Config.Matrix)
			if err != nil {
				return false, &util.JSONResponse{
					Code: http.StatusUnauthorized,
					JSON: spec.InvalidUsername(err.Error()),
				}
			}
			if t.Rt != nil {
				t.Rt.Act(localpart)
			}
			return false, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden("The username or password was incorrect or the account does not exist."),
			}
		}
	} else {
		bindDn := strings.ReplaceAll(t.Config.Ldap.UserBindDn, "{username}", username)
		err = conn.Bind(bindDn, password)
		if err != nil {
			var localpart string
			localpart, _, err = userutil.ParseUsernameParam(username, t.Config.Matrix)
			if err != nil {
				return false, &util.JSONResponse{
					Code: http.StatusUnauthorized,
					JSON: spec.InvalidUsername(err.Error()),
				}
			}
			if t.Rt != nil {
				t.Rt.Act(localpart)
			}
			return false, &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden("The username or password was incorrect or the account does not exist."),
			}
		}
	}

	isAdmin, err := t.isLdapAdmin(conn, username)
	if err != nil {
		return false, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.InvalidUsername(err.Error()),
		}
	}
	return isAdmin, nil
}

func (t *LoginTypePassword) isLdapAdmin(conn *ldap.Conn, username string) (bool, error) {
	searchRequest := ldap.NewSearchRequest(
		t.Config.Ldap.AdminGroupDn,
		ldap.ScopeWholeSubtree, ldap.DerefAlways, 0, 0, false,
		strings.ReplaceAll(t.Config.Ldap.AdminGroupFilter, "{username}", username),
		[]string{t.Config.Ldap.AdminGroupAttribute},
		nil)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return false, err
	}

	if len(sr.Entries) < 1 {
		return false, nil
	}
	return true, nil
}

func (t *LoginTypePassword) getOrCreateAccount(ctx context.Context, localpart string, domain spec.ServerName, admin bool) (*api.Account, *util.JSONResponse) {
	var existing api.QueryAccountByLocalpartResponse
	err := t.UserLoginAPI.QueryAccountByLocalpart(ctx, &api.QueryAccountByLocalpartRequest{
		Localpart:  localpart,
		ServerName: domain,
	}, &existing)

	if err == nil {
		return existing.Account, nil
	}
	if err != sql.ErrNoRows {
		return nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: spec.InvalidUsername(err.Error()),
		}
	}

	accountType := api.AccountTypeUser
	if admin {
		accountType = api.AccountTypeAdmin
	}
	var created api.PerformAccountCreationResponse
	err = t.UserLoginAPI.PerformAccountCreation(ctx, &api.PerformAccountCreationRequest{
		AppServiceID: "ldap",
		Localpart:    localpart,
		Password:     uuid.New().String(),
		AccountType:  accountType,
		OnConflict:   api.ConflictAbort,
	}, &created)

	if err != nil {
		if _, ok := err.(*api.ErrorConflict); ok {
			return nil, &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.UserInUse("Desired user ID is already taken."),
			}
		}
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("failed to create account: " + err.Error()),
		}
	}
	return created.Account, nil
}
