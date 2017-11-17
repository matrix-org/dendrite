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
	"net/http"
	"net/url"
	"regexp"
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
)

var validUsernameRegex = regexp.MustCompile(`^[0-9a-zA-Z_\-./]+$`)

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
	Flows     []authFlow             `json:"flows"`
	Completed []authtypes.LoginType  `json:"completed"`
	Params    map[string]interface{} `json:"params"`
	Session   string                 `json:"session"`
}

// authFlow represents one possible way that the client can authenticate a request.
// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#user-interactive-authentication-api
type authFlow struct {
	Stages []authtypes.LoginType `json:"stages"`
}

// legacyRegisterRequest represents the submitted registration request for v1 API.
type legacyRegisterRequest struct {
	Password string                      `json:"password"`
	Username string                      `json:"user"`
	Admin    bool                        `json:"admin"`
	Type     authtypes.LoginType         `json:"type"`
	Mac      gomatrixserverlib.HexString `json:"mac"`
}

func newUserInteractiveResponse(sessionID string, fs []authFlow) userInteractiveResponse {
	return userInteractiveResponse{
		fs, []authtypes.LoginType{}, make(map[string]interface{}), sessionID,
	}
}

// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
type registerResponse struct {
	UserID      string                       `json:"user_id"`
	AccessToken string                       `json:"access_token"`
	HomeServer  gomatrixserverlib.ServerName `json:"home_server"`
	DeviceID    string                       `json:"device_id"`
}

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

	defer resp.Body.Close()

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

	// All registration requests must specify what auth they are using to perform this request
	if r.Auth.Type == "" {
		return util.JSONResponse{
			Code: 401,
			// TODO: Hard-coded 'dummy' auth for now with a bogus session ID.
			//       Server admins should be able to change things around (eg enable captcha)
			JSON: newUserInteractiveResponse(time.Now().String(), []authFlow{
				{[]authtypes.LoginType{authtypes.LoginTypeDummy}},
				{[]authtypes.LoginType{authtypes.LoginTypeRecaptcha}},
				{[]authtypes.LoginType{authtypes.LoginTypeSharedSecret}},
			}),
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
		if !cfg.Matrix.RegistrationRecaptcha {
			return util.MessageResponse(400, "Captcha registration is disabled")
		}

		logger := util.GetLogger(req.Context())
		logger.WithFields(log.Fields{
			"clientip": req.RemoteAddr,
			"response": r.Auth.Response,
		}).Info("Submitting recaptcha response")

		// Check given captcha response
		if resErr = validateRecaptcha(cfg, r.Auth.Response, req.RemoteAddr); resErr != nil {
			return *resErr
		}
		return completeRegistration(req.Context(), accountDB, deviceDB, r.Username, r.Password)

	case authtypes.LoginTypeSharedSecret:
		if cfg.Matrix.RegistrationSharedSecret == "" {
			return util.MessageResponse(400, "Shared secret registration is disabled")
		}

		valid, err := isValidMacLogin(r.Username, r.Password, r.Admin, r.Auth.Mac, cfg.Matrix.RegistrationSharedSecret)

		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if !valid {
			return util.MessageResponse(403, "HMAC incorrect")
		}

		return completeRegistration(req.Context(), accountDB, deviceDB, r.Username, r.Password)
	case authtypes.LoginTypeDummy:
		// there is nothing to do
		return completeRegistration(req.Context(), accountDB, deviceDB, r.Username, r.Password)
	default:
		return util.JSONResponse{
			Code: 501,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
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

		return completeRegistration(req.Context(), accountDB, deviceDB, r.Username, r.Password)
	case authtypes.LoginTypeDummy:
		// there is nothing to do
		return completeRegistration(req.Context(), accountDB, deviceDB, r.Username, r.Password)
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
	dev, err := deviceDB.CreateDevice(ctx, username, nil, token)
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
