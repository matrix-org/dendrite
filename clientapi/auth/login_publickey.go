// Copyright 2022 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/mapsutil"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
)

type LoginPublicKeyHandler interface {
	AccountExists(ctx context.Context) (string, *jsonerror.MatrixError)
	IsValidUserIdForRegistration(userId string) bool
	CreateLogin() *Login
	GetSession() string
	GetType() string
	ValidateLoginResponse() (bool, *jsonerror.MatrixError)
}

// LoginTypePublicKey implements https://matrix.org/docs/spec/client_server/..... (to be spec'ed)
type LoginTypePublicKey struct {
	UserAPI         userapi.ClientUserAPI
	UserInteractive *UserInteractive
	Config          *config.ClientAPI
}

func (t *LoginTypePublicKey) Name() string {
	return authtypes.LoginTypePublicKey
}

func (t *LoginTypePublicKey) AddFlows(userInteractive *UserInteractive) {
	if t.Config.PublicKeyAuthentication.Ethereum.Enabled {
		userInteractive.Flows = append(userInteractive.Flows, userInteractiveFlow{
			Stages: []string{
				authtypes.LoginTypePublicKeyEthereum,
			},
		})
		params := t.Config.PublicKeyAuthentication.GetPublicKeyRegistrationParams()
		userInteractive.Params = mapsutil.MapsUnion(userInteractive.Params, params)
	}

	if t.Config.PublicKeyAuthentication.Enabled() {
		userInteractive.Types[t.Name()] = t
	}
}

// LoginFromJSON implements Type.
func (t *LoginTypePublicKey) LoginFromJSON(ctx context.Context, reqBytes []byte) (*Login, LoginCleanupFunc, *util.JSONResponse) {
	// "A client should first make a request with no auth parameter. The homeserver returns an HTTP 401 response, with a JSON body"
	// https://matrix.org/docs/spec/client_server/r0.6.1#user-interactive-api-in-the-rest-api
	authBytes := gjson.GetBytes(reqBytes, "auth")
	if !authBytes.Exists() {
		return nil, nil, t.UserInteractive.NewSession()
	}

	var authHandler LoginPublicKeyHandler
	authType := gjson.GetBytes(reqBytes, "auth.type").String()

	switch authType {
	case authtypes.LoginTypePublicKeyEthereum:
		pkEthHandler, err := CreatePublicKeyEthereumHandler(
			[]byte(authBytes.Raw),
			t.UserAPI,
			t.Config,
		)
		if err != nil {
			return nil, nil, &util.JSONResponse{
				Code: http.StatusUnauthorized,
				JSON: err,
			}
		}
		authHandler = *pkEthHandler
	default:
		return nil, nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.InvalidParam("auth.type"),
		}
	}

	return t.continueLoginFlow(ctx, authHandler)
}

func (t *LoginTypePublicKey) continueLoginFlow(ctx context.Context, authHandler LoginPublicKeyHandler) (*Login, LoginCleanupFunc, *util.JSONResponse) {
	loginOK := false
	sessionID := authHandler.GetSession()

	defer func() {
		if loginOK {
			t.UserInteractive.AddCompletedStage(sessionID, authHandler.GetType())
		} else {
			t.UserInteractive.DeleteSession(sessionID)
		}
	}()

	if _, ok := t.UserInteractive.Sessions[sessionID]; !ok {
		return nil, nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.Unknown("the session ID is missing or unknown."),
		}
	}

	localPart, err := authHandler.AccountExists(ctx)
	// user account does not exist or there is an error.
	if localPart == "" || err != nil {
		return nil, nil, &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: err,
		}
	}

	// user account exists
	isValidated, err := authHandler.ValidateLoginResponse()
	if err != nil {
		return nil, nil, &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: err,
		}
	}

	if isValidated {
		loginOK = true
		login := authHandler.CreateLogin()
		return login, func(context.Context, *util.JSONResponse) {}, nil
	}

	return nil, nil, &util.JSONResponse{
		Code: http.StatusUnauthorized,
		JSON: jsonerror.Unknown("authentication failed, or the account does not exist."),
	}
}
