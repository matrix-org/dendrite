// Copyright 2017 Vector Creations Ltd
// Copyright 2017 New Vector Ltd
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
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
)

type authDict struct {
	Type    authtypes.LoginType         `json:"type"`
	Session string                      `json:"session"`
	Mac     gomatrixserverlib.HexString `json:"mac"`

	// Recaptcha
	Response string `json:"response"`
	// TODO: Lots of custom keys depending on the type
}

// a generic flowRequest type for any auth call to UIAA handler
type userInteractiveFlowRequest struct {
	// registration parameters
	Password string `json:"password"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`

	// user-interactive auth params
	Auth authDict `json:"auth"`

	// Application Services place Type in the root of their registration
	// request, whereas clients place it in the authDict struct.
	Type authtypes.LoginType `json:"type"`
}

type userInteractiveResponseHandler func(
	*http.Request,
	userInteractiveFlowRequest,
// some other param such as username or appserviceId
	string,
) util.JSONResponse

// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#user-interactive-authentication-api
type userInteractiveResponse struct {
	Flows     []authtypes.Flow       `json:"flows"`
	Completed []authtypes.LoginType  `json:"completed"`
	Params    map[string]interface{} `json:"params"`
	Session   string                 `json:"session"`
}

// newUserInteractiveResponse will return a struct to be sent back to the client
func newUserInteractiveResponse(
	sessionID string,
	fs []authtypes.Flow,
	params map[string]interface{},
) userInteractiveResponse {
	return userInteractiveResponse{
		fs, sessions[sessionID], params, sessionID,
	}
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
			Code: 400,
			JSON: jsonerror.BadJSON("Captcha registration is disabled"),
		}
	}

	if response == "" {
		return &util.JSONResponse{
			Code: 400,
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
			Code: 500,
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
			Code: 500,
			JSON: jsonerror.BadJSON("Error in contacting captcha server" + err.Error()),
		}
	}
	err = json.Unmarshal(body, &r)
	if err != nil {
		return &util.JSONResponse{
			Code: 500,
			JSON: jsonerror.BadJSON("Error in unmarshaling captcha server's response: " + err.Error()),
		}
	}

	// Check that we received a "success"
	if !r.Success {
		return &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.BadJSON("Invalid captcha response. Please try again."),
		}
	}
	return nil
}

// UsernameIsWithinApplicationServiceNamespace checks to see if a username falls
// within any of the namespaces of a given Application Service. If no
// Application Service is given, it will check to see if it matches any
// Application Service's namespace.
func UsernameIsWithinApplicationServiceNamespace(
	cfg *config.Dendrite,
	username string,
	appservice *config.ApplicationService,
) bool {
	if appservice != nil {
		// Loop through given Application Service's namespaces and see if any match
		for _, namespace := range appservice.NamespaceMap["users"] {
			// AS namespaces are checked for validity in config
			if namespace.RegexpObject.MatchString(username) {
				return true
			}
		}
		return false
	}

	// Loop through all known Application Service's namespaces and see if any match
	for _, knownAppservice := range cfg.Derived.ApplicationServices {
		for _, namespace := range knownAppservice.NamespaceMap["users"] {
			// AS namespaces are checked for validity in config
			if namespace.RegexpObject.MatchString(username) {
				return true
			}
		}
	}
	return false
}

// validateApplicationService checks if a provided application service token
// corresponds to one that is registered. If so, then it checks if the desired
// username is within that application service's namespace. As long as these
// two requirements are met, no error will be returned.
func validateApplicationService(
	cfg *config.Dendrite,
	req *http.Request,
	username string,
) (string, *util.JSONResponse) {
	// Check if the token if the application service is valid with one we have
	// registered in the config.
	accessToken := req.URL.Query().Get("access_token")
	var matchedApplicationService *config.ApplicationService
	for _, appservice := range cfg.Derived.ApplicationServices {
		if appservice.ASToken == accessToken {
			matchedApplicationService = &appservice
			break
		}
	}
	if matchedApplicationService != nil {
		return "", &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.UnknownToken("Supplied access_token does not match any known application service"),
		}
	}

	// Ensure the desired username is within at least one of the application service's namespaces.
	if !UsernameIsWithinApplicationServiceNamespace(cfg, username, matchedApplicationService) {
		// If we didn't find any matches, return M_EXCLUSIVE
		return "", &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
			"Supplied username %s did not match any namespaces for application service ID: %s",
			username,
			matchedApplicationService.ID)),
		}
	}

	// Check this user does not fit multiple application service namespaces
	if UsernameMatchesMultipleExclusiveNamespaces(cfg, username) {
		return "", &util.JSONResponse{
			Code: 401,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
				"Supplied username %s matches multiple exclusive application service namespaces. Only 1 match allowed",
				username)),
		}
	}

	// No errors, valid
	return matchedApplicationService.ID, nil
}

// handleUIAAFlow will direct and complete UIAA flow stages
// that the client has requested.
func HandleUserInteractiveFlow (
	req *http.Request,

//the list of allowed flows and params
	r userInteractiveFlowRequest,

	sessionID string,
	cfg *config.Dendrite,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
	res userInteractiveResponse,
	responseHandler userInteractiveResponseHandler,
) util.JSONResponse {

	// TODO: email / msisdn auth types.

	if cfg.Matrix.RegistrationDisabled && r.Auth.Type != authtypes.LoginTypeSharedSecret {
		return util.MessageResponse(403, "Registration has been disabled")
	}

	switch r.Auth.Type {
	case authtypes.LoginTypeRecaptcha:
		// Check given captcha response
		resErr := validateRecaptcha(cfg, r.Auth.Response, req.RemoteAddr)
		if resErr != nil {
			return *resErr
		}

		// Add Recaptcha to the list of completed stages
		sessions[sessionID] = append(sessions[sessionID], authtypes.LoginTypeRecaptcha)

	case authtypes.LoginTypeSharedSecret:
		// Check shared secret against config
		valid, err := isValidMacLogin(cfg, r.Username, r.Password, r.Admin, r.Auth.Mac)

		if err != nil {
			return httputil.LogThenError(req, err)
		} else if !valid {
			return util.MessageResponse(403, "HMAC incorrect")
		}

		// Add SharedSecret to the list of completed stages
		sessions[sessionID] = append(sessions[sessionID], authtypes.LoginTypeSharedSecret)

	case authtypes.LoginTypeApplicationService:
		// Check Application Service register user request is valid.
		// The application service's ID is returned if so.
		appserviceID, err := validateApplicationService(cfg, req, r.Username)

		if err != nil {
			return *err
		}

		// If no error, application service was successfully validated.
		// Don't need to worry about appending to stages as
		// application services are entirely separate.
		return responseHandler(req, r, appserviceID)

	case authtypes.LoginTypeDummy:
		// there is nothing to do
		// Add Dummy to the list of completed stages
		sessions[sessionID] = append(sessions[sessionID], authtypes.LoginTypeDummy)

	default:
		return util.JSONResponse{
			Code: 501,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}

	// Check if the user's flow has been completed successfully
	// A response with current flow and remaining available methods
	// will be returned if a flow has not been successfully completed yet
	return checkAndCompleteFlow(sessions[sessionID], req, r, sessionID, res, responseHandler)
}

// checkAndCompleteFlow checks if a given flow is completed given
// a set of allowed flows. If so, task is completed, otherwise a
// response with
func checkAndCompleteFlow(
	flow []authtypes.LoginType,
	req *http.Request,
	r userInteractiveFlowRequest,
	sessionID string,
	res userInteractiveResponse,
	responseHandler userInteractiveResponseHandler,
) util.JSONResponse {
	if checkFlowCompleted(flow, res.Flows) {
		// This flow was completed, task can continue
		return responseHandler(req, r, "")
	}

	// There are still more stages to complete.
	// Return the flows and those that have been completed.
	return util.JSONResponse{
		Code: 401,
		JSON: newUserInteractiveResponse(sessionID,
			res.Flows, res.Params),
	}
}

// Used for shared secret registration.
// Checks if the username, password and isAdmin flag matches the given mac.
func isValidMacLogin(
	cfg *config.Dendrite,
	username, password string,
	isAdmin bool,
	givenMac []byte,
) (bool, error) {
	sharedSecret := cfg.Matrix.RegistrationSharedSecret

	// Check that shared secret registration isn't disabled.
	if cfg.Matrix.RegistrationSharedSecret == "" {
		return false, errors.New("Shared secret registration is disabled")
	}

	// Double check that username/password don't contain the HMAC delimiters. We should have
	// already checked this.
	if strings.Contains(username, "\x00") {
		return false, errors.New("Username contains invalid character")
	}
	if strings.Contains(password, "\x00") {
		return false, errors.New("Password contains invalid character")
	}
	if sharedSecret == "" {
		return false, errors.New("Shared secret registration is disabled")
	}

	adminString := "notadmin"
	if isAdmin {
		adminString = "admin"
	}
	joined := strings.Join([]string{username, password, adminString}, "\x00")

	mac := hmac.New(sha1.New, []byte(sharedSecret))
	_, err := mac.Write([]byte(joined))
	if err != nil {
		return false, err
	}
	expectedMAC := mac.Sum(nil)

	return hmac.Equal(givenMac, expectedMAC), nil
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

// checkFlowCompleted checks if a flow complies with any allowed flow
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
