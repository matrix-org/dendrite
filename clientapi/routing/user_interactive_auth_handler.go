// Copyright 2019 Vector Creations Ltd
// Copyright 2019 New Vector Ltd
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

package routing

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type AuthDict struct {
	Type    authtypes.LoginType         `json:"type"`
	Session string                      `json:"session"`
	Mac     gomatrixserverlib.HexString `json:"mac"`

	// Recaptcha
	Response string `json:"response"`
	// TODO: Lots of custom keys depending on the type
}

// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#user-interactive-authentication-api
type userInteractiveResponse struct {
	Flows     []authtypes.Flow       `json:"flows"`
	Completed []authtypes.LoginType  `json:"completed"`
	Params    map[string]interface{} `json:"params"`
	Session   string                 `json:"session"`
}

// newUserInteractiveResponse will return a struct to be sent back to the client.
func newUserInteractiveResponse(
	sessionID string,
	fs []authtypes.Flow,
	params map[string]interface{},
) userInteractiveResponse {
	return userInteractiveResponse{
		fs, sessions.GetCompletedStages(sessionID), params, sessionID,
	}
}

// getCompletedStages returns the completed stages for a session.
func (d *sessionsDict) GetCompletedStages(sessionID string) []authtypes.LoginType {
	d.Lock()
	defer d.Unlock()

	if completedStages, ok := d.sessions[sessionID]; ok {
		return completedStages
	}
	// Ensure that a empty slice is returned and not nil. See #399.
	return make([]authtypes.LoginType, 0)
}

// addCompletedSessionStage records that a session has completed an auth stage.
func AddCompletedSessionStage(sessionID string, stage authtypes.LoginType) {
	sessions.Lock()
	defer sessions.Unlock()

	for _, completedStage := range sessions.sessions[sessionID] {
		if completedStage == stage {
			return
		}
	}
	sessions.sessions[sessionID] = append(sessions.sessions[sessionID], stage)
}

// HandleUserInteractiveFlow will direct and complete auth flow stages
// that the client has requested. This function requires http request, session Id
// authentication data as AuthDict object, config object and a config.UserInteractiveAuthConfig
// object containing list of allowed auth flows and params for successful authentication.
// It returns nil if auth is successful else returns jsonResponse (can be sent directly to
// client as http response) containing userInteractiveResponse or relevant error (if encountered).
func HandleUserInteractiveFlow(
	req *http.Request,
	auth AuthDict,
	sessionID string,
	cfg *config.Dendrite,
	authCfg config.UserInteractiveAuthConfig,
) *util.JSONResponse {

	switch auth.Type {
	case authtypes.LoginTypeRecaptcha:
		// Check given captcha response
		resErr := validateRecaptcha(cfg, auth.Response, req.RemoteAddr)
		if resErr != nil {
			return resErr
		}

		// Add Recaptcha to the list of completed authentication stages
		AddCompletedSessionStage(sessionID, authtypes.LoginTypeRecaptcha)

	// A missing auth type means the request is made as the first step of a authentication
	// using the User-Interactive Authentication API. Skip the default case for this.
	case "":

	case authtypes.LoginTypeDummy:
		// there is nothing to do
		// Add Dummy to the list of completed authentication stages
		AddCompletedSessionStage(sessionID, authtypes.LoginTypeDummy)

	default:
		return &util.JSONResponse{
			Code: http.StatusNotImplemented,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}

	// Check if the user's authentication flow has been completed successfully
	// A response with current authentication flow and remaining available methods
	// will be returned if a flow has not been successfully completed yet
	return checkAndCompleteFlow(sessions.GetCompletedStages(sessionID), sessionID, authCfg)
}

// checkAndCompleteFlow checks if authentication flow is completed given
// a set of allowed stages. If so, authentication is completed and
// function returns nil, otherwise a userInteractiveResponse
// with required stages is returned.
func checkAndCompleteFlow(
	flow []authtypes.LoginType,
	sessionID string,
	authCfg config.UserInteractiveAuthConfig,
) *util.JSONResponse {
	if checkFlowCompleted(flow, authCfg.Flows) {
		// This flow was completed, authentication successful.
		return nil
	}

	// There are still more stages to complete.
	// Return the flows and those that have been completed.
	return &util.JSONResponse{
		Code: http.StatusUnauthorized,
		JSON: newUserInteractiveResponse(sessionID,
			authCfg.Flows, authCfg.Params),
	}
}

// checkFlows checks a single completed flow against another required one. If
// one contains at least all of the stages that the other does, checkFlows
// returns true.
func checkFlows(
	completedStages []authtypes.LoginType,
	requiredStages []authtypes.LoginType,
) bool {
	// Create temporary slices so they originals will not be modified on sorting
	completed := make([]authtypes.LoginType, len(completedStages))
	required := make([]authtypes.LoginType, len(requiredStages))
	copy(completed, completedStages)
	copy(required, requiredStages)

	// Sort the slices for simple comparison
	sort.Slice(completed, func(i, j int) bool { return completed[i] < completed[j] })
	sort.Slice(required, func(i, j int) bool { return required[i] < required[j] })

	// Iterate through each slice, going to the next required slice only once
	// we've found a match.
	i, j := 0, 0
	for j < len(required) {
		// Exit if we've reached the end of our input without being able to
		// match all of the required stages.
		if i >= len(completed) {
			return false
		}

		// If we've found a stage we want, move on to the next required stage.
		if completed[i] == required[j] {
			j++
		}
		i++
	}
	return true
}

// checkFlowCompleted checks if a authentication flow complies with any allowed flow
// dictated by the server. Order of stages does not matter. A user may complete
// extra stages as long as the required stages of at least one flow is met.
func checkFlowCompleted(
	flow []authtypes.LoginType,
	allowedFlows []authtypes.Flow,
) bool {
	// Iterate through possible flows to check whether any have been fully completed.
	for _, allowedFlow := range allowedFlows {
		if checkFlows(flow, allowedFlow.Stages) {
			return true
		}
	}
	return false
}

// recaptchaResponse represents the HTTP response from a Google Recaptcha server
type recaptchaResponse struct {
	Success     bool      `json:"success"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []int     `json:"error-codes"`
}

// validateRecaptcha returns an error response if the captcha response is invalid
func validateRecaptcha(
	cfg *config.Dendrite,
	response string,
	clientip string,
) *util.JSONResponse {
	if !cfg.Matrix.RecaptchaEnabled {
		return &util.JSONResponse{
			Code: http.StatusConflict,
			JSON: jsonerror.Unknown("Captcha authentication is disabled"),
		}
	}

	if response == "" {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Captcha response is required"),
		}
	}

	// Make a POST request to Google's API to check the captcha response
	resp, err := http.PostForm(cfg.Matrix.RecaptchaSiteVerifyAPI,
		url.Values{
			"secret":   {cfg.Matrix.RecaptchaPrivateKey},
			"response": {response},
			"remoteip": {clientip},
		},
	)

	if err != nil {
		return &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.BadJSON("Error in requesting validation of captcha response"),
		}
	}

	// Close the request once we're finishing reading from it
	defer resp.Body.Close() // nolint: errcheck

	// Grab the body of the response from the captcha server
	var r recaptchaResponse
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return &util.JSONResponse{
			Code: http.StatusGatewayTimeout,
			JSON: jsonerror.Unknown("Error in contacting captcha server" + err.Error()),
		}
	}
	err = json.Unmarshal(body, &r)
	if err != nil {
		return &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.BadJSON("Error in unmarshaling captcha server's response: " + err.Error()),
		}
	}

	// Check that we received a "success"
	if !r.Success {
		return &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.BadJSON("Invalid captcha response. Please try again."),
		}
	}
	return nil
}

type availableResponse struct {
	Available bool `json:"available"`
}
