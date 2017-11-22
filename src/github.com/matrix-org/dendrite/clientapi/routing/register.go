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
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/matrix-org/dendrite/common/config"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

const (
	minPasswordLength = 8   // http://matrix.org/docs/spec/client_server/r0.2.0.html#password-based
	maxPasswordLength = 512 // https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	maxUsernameLength = 254 // http://matrix.org/speculator/spec/HEAD/intro.html#user-identifiers TODO account for domain
	sessionIDLength   = 24
)

var (
	sessions           = make(map[string][]authtypes.LoginType) // Sessions and completed flow stages
	validUsernameRegex = regexp.MustCompile(`^[0-9a-zA-Z_\-./]+$`)
)

// registerRequest represents the submitted registration request.
// It can be broken down into 2 sections: the auth dictionary and registration parameters.
// Registration parameters vary depending on the request, and will need to remembered across
// sessions. If no parameters are supplied, the server should use the parameters previously
// remembered. If ANY parameters are supplied, the server should REPLACE all knowledge of
// previous parameters with the ones supplied. This mean you cannot "build up" request params.
type registerRequest struct {
	// registration parameters.
	Password string `json:"password"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
	// user-interactive auth params
	Auth authDict `json:"auth"`

	InitialDisplayName *string `json:"initial_device_display_name"`
}

type authDict struct {
	Type    authtypes.LoginType         `json:"type"`
	Session string                      `json:"session"`
	Mac     gomatrixserverlib.HexString `json:"mac"`

	// ReCaptcha
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

// legacyRegisterRequest represents the submitted registration request for v1 API.
type legacyRegisterRequest struct {
	Password string                      `json:"password"`
	Username string                      `json:"user"`
	Admin    bool                        `json:"admin"`
	Type     authtypes.LoginType         `json:"type"`
	Mac      gomatrixserverlib.HexString `json:"mac"`
}

func newUserInteractiveResponse(sessionID string, fs []authtypes.Flow) userInteractiveResponse {
	return userInteractiveResponse{
		fs, sessions[sessionID], make(map[string]interface{}), sessionID,
	}
}

// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
type registerResponse struct {
	UserID      string                       `json:"user_id"`
	AccessToken string                       `json:"access_token"`
	HomeServer  gomatrixserverlib.ServerName `json:"home_server"`
	DeviceID    string                       `json:"device_id"`
}

// recaptchaResponse represents the HTTP response from a Google ReCaptcha server
type recaptchaResponse struct {
	Success     bool      `json:"success"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []int     `json:"error-codes"`
}

// validateUserName returns an error response if the username is invalid
func validateUserName(username string) *util.JSONResponse {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if len(username) > maxUsernameLength {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'username' >%d characters", maxUsernameLength)),
		}
	} else if !validUsernameRegex.MatchString(username) {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidUsername("User ID can only contain characters a-z, 0-9, or '_-./'"),
		}
	} else if username[0] == '_' { // Regex checks its not a zero length string
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidUsername("User ID can't start with a '_'"),
		}
	}
	return nil
}

// validatePassword returns an error response if the password is invalid
func validatePassword(password string) *util.JSONResponse {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if len(password) > maxPasswordLength {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'password' >%d characters", maxPasswordLength)),
		}
	} else if len(password) > 0 && len(password) < minPasswordLength {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.WeakPassword(fmt.Sprintf("password too weak: min %d chars", minPasswordLength)),
		}
	}
	return nil
}

// validateRecaptcha returns an error response if the captcha response is invalid
func validateRecaptcha(
	req *http.Request,
	cfg *config.Dendrite,
	response string,
	clientip string,
) *util.JSONResponse {
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
	defer resp.Body.Close() // noline: errcheck

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

// TODO: Create flows in config.go so that they're cached. Always show msisdn flows as long as flows only depend on config-file options.
// Store it just like the config does in a struct and keep it there.

// Register processes a /register request. http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
func Register(
	req *http.Request,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
	cfg *config.Dendrite,
) util.JSONResponse {

	var r registerRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	// Retrieve or generate the sessionID
	sessionID := r.Auth.Session
	if sessionID == "" {
		// Generate a new, random session ID
		sessionID = RandString(sessionIDLength)
	}

	// If no auth type is specified by the client, send back the list of available flows
	if r.Auth.Type == "" {
		return util.JSONResponse{
			Code: 401,
			JSON: newUserInteractiveResponse(sessionID, cfg.Derived.Flows),
		}
	}

	if resErr = validateUserName(r.Username); resErr != nil {
		return *resErr
	}
	if resErr = validatePassword(r.Password); resErr != nil {
		return *resErr
	}

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"username":   r.Username,
		"auth.type":  r.Auth.Type,
		"session_id": r.Auth.Session,
	}).Info("Processing registration request")

	// TODO: Shared secret registration (create new user scripts)
	// TODO: AS API registration
	// TODO: Enable registration config flag
	// TODO: Guest account upgrading

	// TODO: Handle loading of previous session parameters from database.
	// TODO: Handle mapping registrationRequest parameters into session parameters

	// TODO: email / msisdn auth types.
	switch r.Auth.Type {
	case authtypes.LoginTypeRecaptcha:
		if !cfg.Matrix.RecaptchaEnabled {
			return util.MessageResponse(400, "Captcha registration is disabled")
		}

		logger := util.GetLogger(req.Context())
		logger.WithFields(log.Fields{
			"clientip": req.RemoteAddr,
			"response": r.Auth.Response,
		}).Info("Submitting recaptcha response")

		// Check given captcha response
		if resErr = validateRecaptcha(req, cfg, r.Auth.Response, req.RemoteAddr); resErr != nil {
			return *resErr
		}

		// Add Recaptcha to the list of completed registration stages
		sessions[sessionID] = append(sessions[sessionID], authtypes.LoginTypeRecaptcha)

	case authtypes.LoginTypeSharedSecret:
		if cfg.Matrix.RegistrationSharedSecret == "" {
			return util.MessageResponse(400, "Shared secret registration is disabled")
		}

		valid, err := isValidMacLogin(r.Username, r.Password, r.Admin,
			r.Auth.Mac, cfg.Matrix.RegistrationSharedSecret)

		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if !valid {
			return util.MessageResponse(403, "HMAC incorrect")
		}

		// Add SharedSecret to the list of completed registration stages
		sessions[sessionID] = append(sessions[sessionID], authtypes.LoginTypeSharedSecret)

	case authtypes.LoginTypeDummy:
		// there is nothing to do
		// Add Dummy to the list of completed registration stages
		sessions[sessionID] = append(sessions[sessionID], authtypes.LoginTypeDummy)

	default:
		return util.JSONResponse{
			Code: 501,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}

	// Check if a registration flow has been completed successfully
	for _, flow := range cfg.Derived.Flows {
		if checkFlowsEqual(flow, authtypes.Flow{sessions[sessionID]}) {
			return completeRegistration(req.Context(), accountDB, deviceDB,
				r.Username, r.Password, r.InitialDisplayName)
		}
	}

	// There are still more stages to complete.
	// Return the flows and those that have been completed.
	return util.JSONResponse{
		Code: 401,
		JSON: newUserInteractiveResponse(sessionID, cfg.Derived.Flows),
	}
}

// LegacyRegister process register requests from the legacy v1 API
func LegacyRegister(
	req *http.Request,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
	cfg *config.Dendrite,
) util.JSONResponse {
	var r legacyRegisterRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}
	if resErr = validateUserName(r.Username); resErr != nil {
		return *resErr
	}
	if resErr = validatePassword(r.Password); resErr != nil {
		return *resErr
	}

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"username":  r.Username,
		"auth.type": r.Type,
	}).Info("Processing registration request")

	// All registration requests must specify what auth they are using to perform this request
	if r.Type == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("invalid type"),
		}
	}

	switch r.Type {
	case authtypes.LoginTypeSharedSecret:
		if cfg.Matrix.RegistrationSharedSecret == "" {
			return util.MessageResponse(400, "Shared secret registration is disabled")
		}

		valid, err := isValidMacLogin(r.Username, r.Password, r.Admin, r.Mac, cfg.Matrix.RegistrationSharedSecret)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if !valid {
			return util.MessageResponse(403, "HMAC incorrect")
		}

		return completeRegistration(req.Context(), accountDB, deviceDB, r.Username, r.Password, nil)
	case authtypes.LoginTypeDummy:
		// there is nothing to do
		return completeRegistration(req.Context(), accountDB, deviceDB, r.Username, r.Password, nil)
	default:
		return util.JSONResponse{
			Code: 501,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}
}

func completeRegistration(
	ctx context.Context,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
	username, password string,
	displayName *string,
) util.JSONResponse {
	if username == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("missing username"),
		}
	}
	if password == "" {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("missing password"),
		}
	}

	acc, err := accountDB.CreateAccount(ctx, username, password)
	if err != nil {
		return util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("failed to create account: " + err.Error()),
		}
	}

	token, err := auth.GenerateAccessToken()
	if err != nil {
		return util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("Failed to generate access token"),
		}
	}

	// // TODO: Use the device ID in the request.
	dev, err := deviceDB.CreateDevice(ctx, username, nil, token, displayName)
	if err != nil {
		return util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: registerResponse{
			UserID:      dev.UserID,
			AccessToken: dev.AccessToken,
			HomeServer:  acc.ServerName,
			DeviceID:    dev.ID,
		},
	}
}

// Used for shared secret registration.
// Checks if the username, password and isAdmin flag matches the given mac.
func isValidMacLogin(
	username, password string,
	isAdmin bool,
	givenMac []byte,
	sharedSecret string,
) (bool, error) {
	// Double check that username/passowrd don't contain the HMAC delimiters. We should have
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

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

// RandString returns a random string of characters with a given length.
// Do note that it is not thread-safe in its current form.
// https://stackoverflow.com/a/31832326
var src = rand.NewSource(time.Now().UnixNano())

func RandString(n int) string {
	b := make([]byte, n)

	// A src.Int63() generates 63 random bits
	for i, cache, remain := n-1, src.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = src.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			b[i] = letterBytes[idx]
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return string(b)
}

// checkFlowsEqual checks if two registration flows have the same stages
// within them. Order of stages does not matter.
func checkFlowsEqual(aFlow, bFlow authtypes.Flow) bool {
	a := aFlow.Stages
	b := bFlow.Stages
	if len(a) != len(b) {
		return false
	}

	a_copy := make([]string, len(a))
	b_copy := make([]string, len(b))

	for loginType := range a {
		a_copy = append(a_copy, string(loginType))
	}
	for loginType := range b {
		b_copy = append(b_copy, string(loginType))
	}

	sort.Strings(a_copy)
	sort.Strings(b_copy)

	return reflect.DeepEqual(a_copy, b_copy)
}

type availableResponse struct {
	Available bool `json:"available"`
}

// RegisterAvailable checks if the username is already taken or invalid
func RegisterAvailable(
	req *http.Request,
	accountDB *accounts.Database,
) util.JSONResponse {
	username := req.URL.Query().Get("username")

	if err := validateUserName(username); err != nil {
		return *err
	}

	availability, availabilityErr := accountDB.CheckAccountAvailability(req.Context(), username)
	if availabilityErr != nil {
		return util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("failed to check availability: " + availabilityErr.Error()),
		}
	}
	if !availability {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.InvalidUsername("A different user ID has already been registered for this session"),
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: availableResponse{
			Available: true,
		},
	}
}
