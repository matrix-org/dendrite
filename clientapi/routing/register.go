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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/eventutil"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/tokens"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

var (
	// Prometheus metrics
	amtRegUsers = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "dendrite_clientapi_reg_users_total",
			Help: "Total number of registered users",
		},
	)
)

const (
	minPasswordLength = 8   // http://matrix.org/docs/spec/client_server/r0.2.0.html#password-based
	maxPasswordLength = 512 // https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	maxUsernameLength = 254 // http://matrix.org/speculator/spec/HEAD/intro.html#user-identifiers TODO account for domain
	sessionIDLength   = 24
)

func init() {
	// Register prometheus metrics. They must be registered to be exposed.
	prometheus.MustRegister(amtRegUsers)
}

// sessionsDict keeps track of completed auth stages for each session.
// It shouldn't be passed by value because it contains a mutex.
type sessionsDict struct {
	sync.Mutex
	sessions map[string][]authtypes.LoginType
}

// GetCompletedStages returns the completed stages for a session.
func (d *sessionsDict) GetCompletedStages(sessionID string) []authtypes.LoginType {
	d.Lock()
	defer d.Unlock()

	if completedStages, ok := d.sessions[sessionID]; ok {
		return completedStages
	}
	// Ensure that a empty slice is returned and not nil. See #399.
	return make([]authtypes.LoginType, 0)
}

func newSessionsDict() *sessionsDict {
	return &sessionsDict{
		sessions: make(map[string][]authtypes.LoginType),
	}
}

// AddCompletedSessionStage records that a session has completed an auth stage.
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

var (
	// TODO: Remove old sessions. Need to do so on a session-specific timeout.
	// sessions stores the completed flow stages for all sessions. Referenced using their sessionID.
	sessions           = newSessionsDict()
	validUsernameRegex = regexp.MustCompile(`^[0-9a-z_\-./]+$`)
)

// registerRequest represents the submitted registration request.
// It can be broken down into 2 sections: the auth dictionary and registration parameters.
// Registration parameters vary depending on the request, and will need to remembered across
// sessions. If no parameters are supplied, the server should use the parameters previously
// remembered. If ANY parameters are supplied, the server should REPLACE all knowledge of
// previous parameters with the ones supplied. This mean you cannot "build up" request params.
type registerRequest struct {
	// registration parameters
	Password string `json:"password"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
	// user-interactive auth params
	Auth authDict `json:"auth"`

	// Both DeviceID and InitialDisplayName can be omitted, or empty strings ("")
	// Thus a pointer is needed to differentiate between the two
	InitialDisplayName *string `json:"initial_device_display_name"`
	DeviceID           *string `json:"device_id"`

	// Prevent this user from logging in
	InhibitLogin eventutil.WeakBoolean `json:"inhibit_login"`

	// Application Services place Type in the root of their registration
	// request, whereas clients place it in the authDict struct.
	Type authtypes.LoginType `json:"type"`
}

type authDict struct {
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

// legacyRegisterRequest represents the submitted registration request for v1 API.
type legacyRegisterRequest struct {
	Password string                      `json:"password"`
	Username string                      `json:"user"`
	Admin    bool                        `json:"admin"`
	Type     authtypes.LoginType         `json:"type"`
	Mac      gomatrixserverlib.HexString `json:"mac"`
}

// newUserInteractiveResponse will return a struct to be sent back to the client
// during registration.
func newUserInteractiveResponse(
	sessionID string,
	fs []authtypes.Flow,
	params map[string]interface{},
) userInteractiveResponse {
	return userInteractiveResponse{
		fs, sessions.GetCompletedStages(sessionID), params, sessionID,
	}
}

// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
type registerResponse struct {
	UserID      string                       `json:"user_id"`
	AccessToken string                       `json:"access_token,omitempty"`
	HomeServer  gomatrixserverlib.ServerName `json:"home_server"`
	DeviceID    string                       `json:"device_id,omitempty"`
}

// recaptchaResponse represents the HTTP response from a Google Recaptcha server
type recaptchaResponse struct {
	Success     bool      `json:"success"`
	ChallengeTS time.Time `json:"challenge_ts"`
	Hostname    string    `json:"hostname"`
	ErrorCodes  []int     `json:"error-codes"`
}

// validateUsername returns an error response if the username is invalid
func validateUsername(username string) *util.JSONResponse {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if len(username) > maxUsernameLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'username' >%d characters", maxUsernameLength)),
		}
	} else if !validUsernameRegex.MatchString(username) {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Username can only contain characters a-z, 0-9, or '_-./'"),
		}
	} else if username[0] == '_' { // Regex checks its not a zero length string
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Username cannot start with a '_'"),
		}
	}
	return nil
}

// validateApplicationServiceUsername returns an error response if the username is invalid for an application service
func validateApplicationServiceUsername(username string) *util.JSONResponse {
	if len(username) > maxUsernameLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'username' >%d characters", maxUsernameLength)),
		}
	} else if !validUsernameRegex.MatchString(username) {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Username can only contain characters a-z, 0-9, or '_-./'"),
		}
	}
	return nil
}

// validatePassword returns an error response if the password is invalid
func validatePassword(password string) *util.JSONResponse {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if len(password) > maxPasswordLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'password' >%d characters", maxPasswordLength)),
		}
	} else if len(password) > 0 && len(password) < minPasswordLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.WeakPassword(fmt.Sprintf("password too weak: min %d chars", minPasswordLength)),
		}
	}
	return nil
}

// validateRecaptcha returns an error response if the captcha response is invalid
func validateRecaptcha(
	cfg *config.ClientAPI,
	response string,
	clientip string,
) *util.JSONResponse {
	if !cfg.RecaptchaEnabled {
		return &util.JSONResponse{
			Code: http.StatusConflict,
			JSON: jsonerror.Unknown("Captcha registration is disabled"),
		}
	}

	if response == "" {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Captcha response is required"),
		}
	}

	// Make a POST request to Google's API to check the captcha response
	resp, err := http.PostForm(cfg.RecaptchaSiteVerifyAPI,
		url.Values{
			"secret":   {cfg.RecaptchaPrivateKey},
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

// UserIDIsWithinApplicationServiceNamespace checks to see if a given userID
// falls within any of the namespaces of a given Application Service. If no
// Application Service is given, it will check to see if it matches any
// Application Service's namespace.
func UserIDIsWithinApplicationServiceNamespace(
	cfg *config.ClientAPI,
	userID string,
	appservice *config.ApplicationService,
) bool {
	if appservice != nil {
		// Loop through given application service's namespaces and see if any match
		for _, namespace := range appservice.NamespaceMap["users"] {
			// AS namespaces are checked for validity in config
			if namespace.RegexpObject.MatchString(userID) {
				return true
			}
		}
		return false
	}

	// Loop through all known application service's namespaces and see if any match
	for _, knownAppService := range cfg.Derived.ApplicationServices {
		for _, namespace := range knownAppService.NamespaceMap["users"] {
			// AS namespaces are checked for validity in config
			if namespace.RegexpObject.MatchString(userID) {
				return true
			}
		}
	}
	return false
}

// UsernameMatchesMultipleExclusiveNamespaces will check if a given username matches
// more than one exclusive namespace. More than one is not allowed
func UsernameMatchesMultipleExclusiveNamespaces(
	cfg *config.ClientAPI,
	username string,
) bool {
	userID := userutil.MakeUserID(username, cfg.Matrix.ServerName)

	// Check namespaces and see if more than one match
	matchCount := 0
	for _, appservice := range cfg.Derived.ApplicationServices {
		if appservice.OwnsNamespaceCoveringUserId(userID) {
			if matchCount++; matchCount > 1 {
				return true
			}
		}
	}
	return false
}

// UsernameMatchesExclusiveNamespaces will check if a given username matches any
// application service's exclusive users namespace
func UsernameMatchesExclusiveNamespaces(
	cfg *config.ClientAPI,
	username string,
) bool {
	userID := userutil.MakeUserID(username, cfg.Matrix.ServerName)
	return cfg.Derived.ExclusiveApplicationServicesUsernameRegexp.MatchString(userID)
}

// validateApplicationService checks if a provided application service token
// corresponds to one that is registered. If so, then it checks if the desired
// username is within that application service's namespace. As long as these
// two requirements are met, no error will be returned.
func validateApplicationService(
	cfg *config.ClientAPI,
	username string,
	accessToken string,
) (string, *util.JSONResponse) {
	// Check if the token if the application service is valid with one we have
	// registered in the config.
	var matchedApplicationService *config.ApplicationService
	for _, appservice := range cfg.Derived.ApplicationServices {
		if appservice.ASToken == accessToken {
			matchedApplicationService = &appservice
			break
		}
	}
	if matchedApplicationService == nil {
		return "", &util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.UnknownToken("Supplied access_token does not match any known application service"),
		}
	}

	userID := userutil.MakeUserID(username, cfg.Matrix.ServerName)

	// Ensure the desired username is within at least one of the application service's namespaces.
	if !UserIDIsWithinApplicationServiceNamespace(cfg, userID, matchedApplicationService) {
		// If we didn't find any matches, return M_EXCLUSIVE
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
				"Supplied username %s did not match any namespaces for application service ID: %s", username, matchedApplicationService.ID)),
		}
	}

	// Check this user does not fit multiple application service namespaces
	if UsernameMatchesMultipleExclusiveNamespaces(cfg, userID) {
		return "", &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive(fmt.Sprintf(
				"Supplied username %s matches multiple exclusive application service namespaces. Only 1 match allowed", username)),
		}
	}

	// Check username application service is trying to register is valid
	if err := validateApplicationServiceUsername(username); err != nil {
		return "", err
	}

	// No errors, registration valid
	return matchedApplicationService.ID, nil
}

// Register processes a /register request.
// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
func Register(
	req *http.Request,
	userAPI userapi.UserInternalAPI,
	accountDB accounts.Database,
	cfg *config.ClientAPI,
) util.JSONResponse {
	var r registerRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}
	if req.URL.Query().Get("kind") == "guest" {
		return handleGuestRegistration(req, r, cfg, userAPI)
	}

	// Retrieve or generate the sessionID
	sessionID := r.Auth.Session
	if sessionID == "" {
		// Generate a new, random session ID
		sessionID = util.RandomString(sessionIDLength)
	}

	// Don't allow numeric usernames less than MAX_INT64.
	if _, err := strconv.ParseInt(r.Username, 10, 64); err == nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Numeric user IDs are reserved"),
		}
	}
	// Auto generate a numeric username if r.Username is empty
	if r.Username == "" {
		id, err := accountDB.GetNewNumericLocalpart(req.Context())
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("accountDB.GetNewNumericLocalpart failed")
			return jsonerror.InternalServerError()
		}

		r.Username = strconv.FormatInt(id, 10)
	}

	// Squash username to all lowercase letters
	r.Username = strings.ToLower(r.Username)

	if resErr = validateUsername(r.Username); resErr != nil {
		return *resErr
	}
	if resErr = validatePassword(r.Password); resErr != nil {
		return *resErr
	}

	// Make sure normal user isn't registering under an exclusive application
	// service namespace. Skip this check if no app services are registered.
	if r.Auth.Type != authtypes.LoginTypeApplicationService &&
		len(cfg.Derived.ApplicationServices) != 0 &&
		UsernameMatchesExclusiveNamespaces(cfg, r.Username) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive("This username is reserved by an application service."),
		}
	}

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"username":   r.Username,
		"auth.type":  r.Auth.Type,
		"session_id": r.Auth.Session,
	}).Info("Processing registration request")

	return handleRegistrationFlow(req, r, sessionID, cfg, userAPI)
}

func handleGuestRegistration(
	req *http.Request,
	r registerRequest,
	cfg *config.ClientAPI,
	userAPI userapi.UserInternalAPI,
) util.JSONResponse {
	var res userapi.PerformAccountCreationResponse
	err := userAPI.PerformAccountCreation(req.Context(), &userapi.PerformAccountCreationRequest{
		AccountType: userapi.AccountTypeGuest,
	}, &res)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create account: " + err.Error()),
		}
	}
	token, err := tokens.GenerateLoginToken(tokens.TokenOptions{
		ServerPrivateKey: cfg.Matrix.PrivateKey.Seed(),
		ServerName:       string(res.Account.ServerName),
		UserID:           res.Account.UserID,
	})

	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("Failed to generate access token"),
		}
	}
	//we don't allow guests to specify their own device_id
	var devRes userapi.PerformDeviceCreationResponse
	err = userAPI.PerformDeviceCreation(req.Context(), &userapi.PerformDeviceCreationRequest{
		Localpart:         res.Account.Localpart,
		DeviceDisplayName: r.InitialDisplayName,
		AccessToken:       token,
	}, &devRes)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: registerResponse{
			UserID:      devRes.Device.UserID,
			AccessToken: devRes.Device.AccessToken,
			HomeServer:  res.Account.ServerName,
			DeviceID:    devRes.Device.ID,
		},
	}
}

// handleRegistrationFlow will direct and complete registration flow stages
// that the client has requested.
// nolint: gocyclo
func handleRegistrationFlow(
	req *http.Request,
	r registerRequest,
	sessionID string,
	cfg *config.ClientAPI,
	userAPI userapi.UserInternalAPI,
) util.JSONResponse {
	// TODO: Shared secret registration (create new user scripts)
	// TODO: Enable registration config flag
	// TODO: Guest account upgrading

	// TODO: Handle loading of previous session parameters from database.
	// TODO: Handle mapping registrationRequest parameters into session parameters

	// TODO: email / msisdn auth types.

	if cfg.RegistrationDisabled && r.Auth.Type != authtypes.LoginTypeSharedSecret {
		return util.MessageResponse(http.StatusForbidden, "Registration has been disabled")
	}

	switch r.Auth.Type {
	case authtypes.LoginTypeRecaptcha:
		// Check given captcha response
		resErr := validateRecaptcha(cfg, r.Auth.Response, req.RemoteAddr)
		if resErr != nil {
			return *resErr
		}

		// Add Recaptcha to the list of completed registration stages
		AddCompletedSessionStage(sessionID, authtypes.LoginTypeRecaptcha)

	case authtypes.LoginTypeSharedSecret:
		// Check shared secret against config
		valid, err := isValidMacLogin(cfg, r.Username, r.Password, r.Admin, r.Auth.Mac)

		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("isValidMacLogin failed")
			return jsonerror.InternalServerError()
		} else if !valid {
			return util.MessageResponse(http.StatusForbidden, "HMAC incorrect")
		}

		// Add SharedSecret to the list of completed registration stages
		AddCompletedSessionStage(sessionID, authtypes.LoginTypeSharedSecret)

	case "":
		// Extract the access token from the request, if there's one to extract
		// (which we can know by checking whether the error is nil or not).
		accessToken, err := auth.ExtractAccessToken(req)

		// A missing auth type can mean either the registration is performed by
		// an AS or the request is made as the first step of a registration
		// using the User-Interactive Authentication API. This can be determined
		// by whether the request contains an access token.
		if err == nil {
			return handleApplicationServiceRegistration(
				accessToken, err, req, r, cfg, userAPI,
			)
		}

	case authtypes.LoginTypeApplicationService:
		// Extract the access token from the request.
		accessToken, err := auth.ExtractAccessToken(req)
		// Let the AS registration handler handle the process from here. We
		// don't need a condition on that call since the registration is clearly
		// stated as being AS-related.
		return handleApplicationServiceRegistration(
			accessToken, err, req, r, cfg, userAPI,
		)

	case authtypes.LoginTypeDummy:
		// there is nothing to do
		// Add Dummy to the list of completed registration stages
		AddCompletedSessionStage(sessionID, authtypes.LoginTypeDummy)

	default:
		return util.JSONResponse{
			Code: http.StatusNotImplemented,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}

	// Check if the user's registration flow has been completed successfully
	// A response with current registration flow and remaining available methods
	// will be returned if a flow has not been successfully completed yet
	return checkAndCompleteFlow(sessions.GetCompletedStages(sessionID),
		req, r, sessionID, cfg, userAPI)
}

// handleApplicationServiceRegistration handles the registration of an
// application service's user by validating the AS from its access token and
// registering the user. Its two first parameters must be the two return values
// of the auth.ExtractAccessToken function.
// Returns an error if the access token couldn't be extracted from the request
// at an earlier step of the registration workflow, or if the provided access
// token doesn't belong to a valid AS, or if there was an issue completing the
// registration process.
func handleApplicationServiceRegistration(
	accessToken string,
	tokenErr error,
	req *http.Request,
	r registerRequest,
	cfg *config.ClientAPI,
	userAPI userapi.UserInternalAPI,
) util.JSONResponse {
	// Check if we previously had issues extracting the access token from the
	// request.
	if tokenErr != nil {
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.MissingToken(tokenErr.Error()),
		}
	}

	// Check application service register user request is valid.
	// The application service's ID is returned if so.
	appserviceID, err := validateApplicationService(
		cfg, r.Username, accessToken,
	)
	if err != nil {
		return *err
	}

	// If no error, application service was successfully validated.
	// Don't need to worry about appending to registration stages as
	// application service registration is entirely separate.
	return completeRegistration(
		req.Context(), userAPI, r.Username, "", appserviceID,
		r.InhibitLogin, r.InitialDisplayName, r.DeviceID,
	)
}

// checkAndCompleteFlow checks if a given registration flow is completed given
// a set of allowed flows. If so, registration is completed, otherwise a
// response with
func checkAndCompleteFlow(
	flow []authtypes.LoginType,
	req *http.Request,
	r registerRequest,
	sessionID string,
	cfg *config.ClientAPI,
	userAPI userapi.UserInternalAPI,
) util.JSONResponse {
	if checkFlowCompleted(flow, cfg.Derived.Registration.Flows) {
		// This flow was completed, registration can continue
		return completeRegistration(
			req.Context(), userAPI, r.Username, r.Password, "",
			r.InhibitLogin, r.InitialDisplayName, r.DeviceID,
		)
	}

	// There are still more stages to complete.
	// Return the flows and those that have been completed.
	return util.JSONResponse{
		Code: http.StatusUnauthorized,
		JSON: newUserInteractiveResponse(sessionID,
			cfg.Derived.Registration.Flows, cfg.Derived.Registration.Params),
	}
}

// LegacyRegister process register requests from the legacy v1 API
func LegacyRegister(
	req *http.Request,
	userAPI userapi.UserInternalAPI,
	cfg *config.ClientAPI,
) util.JSONResponse {
	var r legacyRegisterRequest
	resErr := parseAndValidateLegacyLogin(req, &r)
	if resErr != nil {
		return *resErr
	}

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"username":  r.Username,
		"auth.type": r.Type,
	}).Info("Processing registration request")

	if cfg.RegistrationDisabled && r.Type != authtypes.LoginTypeSharedSecret {
		return util.MessageResponse(http.StatusForbidden, "Registration has been disabled")
	}

	switch r.Type {
	case authtypes.LoginTypeSharedSecret:
		if cfg.RegistrationSharedSecret == "" {
			return util.MessageResponse(http.StatusBadRequest, "Shared secret registration is disabled")
		}

		valid, err := isValidMacLogin(cfg, r.Username, r.Password, r.Admin, r.Mac)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("isValidMacLogin failed")
			return jsonerror.InternalServerError()
		}

		if !valid {
			return util.MessageResponse(http.StatusForbidden, "HMAC incorrect")
		}

		return completeRegistration(req.Context(), userAPI, r.Username, r.Password, "", false, nil, nil)
	case authtypes.LoginTypeDummy:
		// there is nothing to do
		return completeRegistration(req.Context(), userAPI, r.Username, r.Password, "", false, nil, nil)
	default:
		return util.JSONResponse{
			Code: http.StatusNotImplemented,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}
}

// parseAndValidateLegacyLogin parses the request into r and checks that the
// request is valid (e.g. valid user names, etc)
func parseAndValidateLegacyLogin(req *http.Request, r *legacyRegisterRequest) *util.JSONResponse {
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return resErr
	}

	// Squash username to all lowercase letters
	r.Username = strings.ToLower(r.Username)

	if resErr = validateUsername(r.Username); resErr != nil {
		return resErr
	}
	if resErr = validatePassword(r.Password); resErr != nil {
		return resErr
	}

	// All registration requests must specify what auth they are using to perform this request
	if r.Type == "" {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("invalid type"),
		}
	}

	return nil
}

// completeRegistration runs some rudimentary checks against the submitted
// input, then if successful creates an account and a newly associated device
// We pass in each individual part of the request here instead of just passing a
// registerRequest, as this function serves requests encoded as both
// registerRequests and legacyRegisterRequests, which share some attributes but
// not all
func completeRegistration(
	ctx context.Context,
	userAPI userapi.UserInternalAPI,
	username, password, appserviceID string,
	inhibitLogin eventutil.WeakBoolean,
	displayName, deviceID *string,
) util.JSONResponse {
	if username == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing username"),
		}
	}
	// Blank passwords are only allowed by registered application services
	if password == "" && appserviceID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing password"),
		}
	}

	var accRes userapi.PerformAccountCreationResponse
	err := userAPI.PerformAccountCreation(ctx, &userapi.PerformAccountCreationRequest{
		AppServiceID: appserviceID,
		Localpart:    username,
		Password:     password,
		AccountType:  userapi.AccountTypeUser,
		OnConflict:   userapi.ConflictAbort,
	}, &accRes)
	if err != nil {
		if _, ok := err.(*userapi.ErrorConflict); ok { // user already exists
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.UserInUse("Desired user ID is already taken."),
			}
		}
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create account: " + err.Error()),
		}
	}

	// Increment prometheus counter for created users
	amtRegUsers.Inc()

	// Check whether inhibit_login option is set. If so, don't create an access
	// token or a device for this user
	if inhibitLogin {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: registerResponse{
				UserID:     userutil.MakeUserID(username, accRes.Account.ServerName),
				HomeServer: accRes.Account.ServerName,
			},
		}
	}

	token, err := auth.GenerateAccessToken()
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("Failed to generate access token"),
		}
	}

	var devRes userapi.PerformDeviceCreationResponse
	err = userAPI.PerformDeviceCreation(ctx, &userapi.PerformDeviceCreationRequest{
		Localpart:         username,
		AccessToken:       token,
		DeviceDisplayName: displayName,
		DeviceID:          deviceID,
	}, &devRes)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: registerResponse{
			UserID:      devRes.Device.UserID,
			AccessToken: devRes.Device.AccessToken,
			HomeServer:  accRes.Account.ServerName,
			DeviceID:    devRes.Device.ID,
		},
	}
}

// Used for shared secret registration.
// Checks if the username, password and isAdmin flag matches the given mac.
func isValidMacLogin(
	cfg *config.ClientAPI,
	username, password string,
	isAdmin bool,
	givenMac []byte,
) (bool, error) {
	sharedSecret := cfg.RegistrationSharedSecret

	// Check that shared secret registration isn't disabled.
	if cfg.RegistrationSharedSecret == "" {
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

// checkFlowCompleted checks if a registration flow complies with any allowed flow
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

type availableResponse struct {
	Available bool `json:"available"`
}

// RegisterAvailable checks if the username is already taken or invalid.
func RegisterAvailable(
	req *http.Request,
	cfg *config.ClientAPI,
	accountDB accounts.Database,
) util.JSONResponse {
	username := req.URL.Query().Get("username")

	// Squash username to all lowercase letters
	username = strings.ToLower(username)

	if err := validateUsername(username); err != nil {
		return *err
	}

	// Check if this username is reserved by an application service
	userID := userutil.MakeUserID(username, cfg.Matrix.ServerName)
	for _, appservice := range cfg.Derived.ApplicationServices {
		if appservice.OwnsNamespaceCoveringUserId(userID) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.UserInUse("Desired user ID is reserved by an application service."),
			}
		}
	}

	availability, availabilityErr := accountDB.CheckAccountAvailability(req.Context(), username)
	if availabilityErr != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to check availability: " + availabilityErr.Error()),
		}
	}
	if !availability {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UserInUse("Desired User ID is already taken."),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: availableResponse{
			Available: true,
		},
	}
}
