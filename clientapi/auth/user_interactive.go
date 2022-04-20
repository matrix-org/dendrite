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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

// Type represents an auth type
// https://matrix.org/docs/spec/client_server/r0.6.1#authentication-types
type Type interface {
	// Name returns the name of the auth type e.g `m.login.password`
	Name() string
	// Login with the auth type, returning an error response on failure.
	// Not all types support login, only m.login.password and m.login.token
	// See https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-login
	// This function will be called when doing login and when doing 'sudo' style
	// actions e.g deleting devices. The response must be a 401 as per:
	// "If the homeserver decides that an attempt on a stage was unsuccessful, but the
	// client may make a second attempt, it returns the same HTTP status 401 response as above,
	// with the addition of the standard errcode and error fields describing the error."
	//
	// The returned cleanup function must be non-nil on success, and will be called after
	// authorization has been completed. Its argument is the final result of authorization.
	LoginFromJSON(ctx context.Context, reqBytes []byte) (login *Login, cleanup LoginCleanupFunc, errRes *util.JSONResponse)
	// TODO: Extend to support Register() flow
	// Register(ctx context.Context, sessionID string, req interface{})
}

type LoginCleanupFunc func(context.Context, *util.JSONResponse)

// LoginIdentifier represents identifier types
// https://matrix.org/docs/spec/client_server/r0.6.1#identifier-types
type LoginIdentifier struct {
	Type string `json:"type"`
	// when type = m.id.user
	User string `json:"user"`
	// when type = m.id.thirdparty
	Medium  string `json:"medium"`
	Address string `json:"address"`
}

// Login represents the shared fields used in all forms of login/sudo endpoints.
type Login struct {
	LoginIdentifier                 // Flat fields deprecated in favour of `identifier`.
	Identifier      LoginIdentifier `json:"identifier"`

	// Both DeviceID and InitialDisplayName can be omitted, or empty strings ("")
	// Thus a pointer is needed to differentiate between the two
	InitialDisplayName *string `json:"initial_device_display_name"`
	DeviceID           *string `json:"device_id"`
}

// Username returns the user localpart/user_id in this request, if it exists.
func (r *Login) Username() string {
	if r.Identifier.Type == "m.id.user" {
		return r.Identifier.User
	}
	// deprecated but without it Element iOS won't log in
	return r.User
}

// ThirdPartyID returns the 3PID medium and address for this login, if it exists.
func (r *Login) ThirdPartyID() (medium, address string) {
	if r.Identifier.Type == "m.id.thirdparty" {
		return r.Identifier.Medium, r.Identifier.Address
	}
	// deprecated
	if r.Medium == "email" {
		return "email", r.Address
	}
	return "", ""
}

type userInteractiveFlow struct {
	Stages []string `json:"stages"`
}

// UserInteractive checks that the user is who they claim to be, via a UI auth.
// This is used for things like device deletion and password reset where
// the user already has a valid access token, but we want to double-check
// that it isn't stolen by re-authenticating them.
type UserInteractive struct {
	Completed []string
	Flows     []userInteractiveFlow
	// Map of login type to implementation
	Types map[string]Type
	// Map of session ID to completed login types, will need to be extended in future
	Sessions map[string][]string
}

func NewUserInteractive(userAccountAPI api.UserAccountAPI, cfg *config.ClientAPI) *UserInteractive {
	typePassword := &LoginTypePassword{
		GetAccountByPassword: userAccountAPI.QueryAccountByPassword,
		Config:               cfg,
	}
	return &UserInteractive{
		Completed: []string{},
		Flows: []userInteractiveFlow{
			{
				Stages: []string{typePassword.Name()},
			},
		},
		Types: map[string]Type{
			typePassword.Name(): typePassword,
		},
		Sessions: make(map[string][]string),
	}
}

func (u *UserInteractive) IsSingleStageFlow(authType string) bool {
	for _, f := range u.Flows {
		if len(f.Stages) == 1 && f.Stages[0] == authType {
			return true
		}
	}
	return false
}

func (u *UserInteractive) AddCompletedStage(sessionID, authType string) {
	// TODO: Handle multi-stage flows
	u.Completed = append(u.Completed, authType)
	delete(u.Sessions, sessionID)
}

type Challenge struct {
	Completed []string              `json:"completed"`
	Flows     []userInteractiveFlow `json:"flows"`
	Session   string                `json:"session"`
	// TODO: Return any additional `params`
	Params map[string]interface{} `json:"params"`
}

// Challenge returns an HTTP 401 with the supported flows for authenticating
func (u *UserInteractive) Challenge(sessionID string) *util.JSONResponse {
	return &util.JSONResponse{
		Code: 401,
		JSON: Challenge{
			Completed: u.Completed,
			Flows:     u.Flows,
			Session:   sessionID,
			Params:    make(map[string]interface{}),
		},
	}
}

// NewSession returns a challenge with a new session ID and remembers the session ID
func (u *UserInteractive) NewSession() *util.JSONResponse {
	sessionID, err := GenerateAccessToken()
	if err != nil {
		logrus.WithError(err).Error("failed to generate session ID")
		res := jsonerror.InternalServerError()
		return &res
	}
	u.Sessions[sessionID] = []string{}
	return u.Challenge(sessionID)
}

// ResponseWithChallenge mixes together a JSON body (e.g an error with errcode/message) with the
// standard challenge response.
func (u *UserInteractive) ResponseWithChallenge(sessionID string, response interface{}) *util.JSONResponse {
	mixedObjects := make(map[string]interface{})
	b, err := json.Marshal(response)
	if err != nil {
		ise := jsonerror.InternalServerError()
		return &ise
	}
	_ = json.Unmarshal(b, &mixedObjects)
	challenge := u.Challenge(sessionID)
	b, err = json.Marshal(challenge.JSON)
	if err != nil {
		ise := jsonerror.InternalServerError()
		return &ise
	}
	_ = json.Unmarshal(b, &mixedObjects)

	return &util.JSONResponse{
		Code: 401,
		JSON: mixedObjects,
	}
}

// Verify returns an error/challenge response to send to the client, or nil if the user is authenticated.
// `bodyBytes` is the HTTP request body which must contain an `auth` key.
// Returns the login that was verified for additional checks if required.
func (u *UserInteractive) Verify(ctx context.Context, bodyBytes []byte, device *api.Device) (*Login, *util.JSONResponse) {
	// TODO: rate limit

	// "A client should first make a request with no auth parameter. The homeserver returns an HTTP 401 response, with a JSON body"
	// https://matrix.org/docs/spec/client_server/r0.6.1#user-interactive-api-in-the-rest-api
	hasResponse := gjson.GetBytes(bodyBytes, "auth").Exists()
	if !hasResponse {
		return nil, u.NewSession()
	}

	// extract the type so we know which login type to use
	authType := gjson.GetBytes(bodyBytes, "auth.type").Str
	loginType, ok := u.Types[authType]
	if !ok {
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Unknown auth.type: " + authType),
		}
	}

	// retrieve the session
	sessionID := gjson.GetBytes(bodyBytes, "auth.session").Str
	if _, ok = u.Sessions[sessionID]; !ok {
		// if the login type is part of a single stage flow then allow them to omit the session ID
		if !u.IsSingleStageFlow(authType) {
			return nil, &util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.Unknown("The auth.session is missing or unknown."),
			}
		}
	}

	login, cleanup, resErr := loginType.LoginFromJSON(ctx, []byte(gjson.GetBytes(bodyBytes, "auth").Raw))
	if resErr != nil {
		return nil, u.ResponseWithChallenge(sessionID, resErr.JSON)
	}

	u.AddCompletedStage(sessionID, authType)
	cleanup(ctx, nil)
	// TODO: Check if there's more stages to go and return an error
	return login, nil
}
