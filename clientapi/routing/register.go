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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/config"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/tokens"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	userapi "github.com/matrix-org/dendrite/userapi/api"
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

// sessionsDict keeps track of completed auth stages for each session.
// It shouldn't be passed by value because it contains a mutex.
type sessionsDict struct {
	sync.RWMutex
	sessions               map[string][]authtypes.LoginType
	sessionCompletedResult map[string]registerResponse
	params                 map[string]registerRequest
	timer                  map[string]*time.Timer
	// deleteSessionToDeviceID protects requests to DELETE /devices/{deviceID} from being abused.
	// If a UIA session is started by trying to delete device1, and then UIA is completed by deleting device2,
	// the delete request will fail for device2 since the UIA was initiated by trying to delete device1.
	deleteSessionToDeviceID map[string]string
}

// defaultTimeout is the timeout used to clean up sessions
const defaultTimeOut = time.Minute * 5

// getCompletedStages returns the completed stages for a session.
func (d *sessionsDict) getCompletedStages(sessionID string) []authtypes.LoginType {
	d.RLock()
	defer d.RUnlock()

	if completedStages, ok := d.sessions[sessionID]; ok {
		return completedStages
	}
	// Ensure that a empty slice is returned and not nil. See #399.
	return make([]authtypes.LoginType, 0)
}

// addParams adds a registerRequest to a sessionID and starts a timer to delete that registerRequest
func (d *sessionsDict) addParams(sessionID string, r registerRequest) {
	d.startTimer(defaultTimeOut, sessionID)
	d.Lock()
	defer d.Unlock()
	d.params[sessionID] = r
}

func (d *sessionsDict) getParams(sessionID string) (registerRequest, bool) {
	d.RLock()
	defer d.RUnlock()
	r, ok := d.params[sessionID]
	return r, ok
}

// deleteSession cleans up a given session, either because the registration completed
// successfully, or because a given timeout (default: 5min) was reached.
func (d *sessionsDict) deleteSession(sessionID string) {
	d.Lock()
	defer d.Unlock()
	delete(d.params, sessionID)
	delete(d.sessions, sessionID)
	delete(d.deleteSessionToDeviceID, sessionID)
	delete(d.sessionCompletedResult, sessionID)
	// stop the timer, e.g. because the registration was completed
	if t, ok := d.timer[sessionID]; ok {
		if !t.Stop() {
			select {
			case <-t.C:
			default:
			}
		}
		delete(d.timer, sessionID)
	}
}

func newSessionsDict() *sessionsDict {
	return &sessionsDict{
		sessions:                make(map[string][]authtypes.LoginType),
		sessionCompletedResult:  make(map[string]registerResponse),
		params:                  make(map[string]registerRequest),
		timer:                   make(map[string]*time.Timer),
		deleteSessionToDeviceID: make(map[string]string),
	}
}

func (d *sessionsDict) startTimer(duration time.Duration, sessionID string) {
	d.Lock()
	defer d.Unlock()
	t, ok := d.timer[sessionID]
	if ok {
		if !t.Stop() {
			<-t.C
		}
		t.Reset(duration)
		return
	}
	d.timer[sessionID] = time.AfterFunc(duration, func() {
		d.deleteSession(sessionID)
	})
}

// addCompletedSessionStage records that a session has completed an auth stage
// also starts a timer to delete the session once done.
func (d *sessionsDict) addCompletedSessionStage(sessionID string, stage authtypes.LoginType) {
	d.startTimer(defaultTimeOut, sessionID)
	d.Lock()
	defer d.Unlock()
	for _, completedStage := range d.sessions[sessionID] {
		if completedStage == stage {
			return
		}
	}
	d.sessions[sessionID] = append(sessions.sessions[sessionID], stage)
}

func (d *sessionsDict) addDeviceToDelete(sessionID, deviceID string) {
	d.startTimer(defaultTimeOut, sessionID)
	d.Lock()
	defer d.Unlock()
	d.deleteSessionToDeviceID[sessionID] = deviceID
}

func (d *sessionsDict) addCompletedRegistration(sessionID string, response registerResponse) {
	d.Lock()
	defer d.Unlock()
	d.sessionCompletedResult[sessionID] = response
}

func (d *sessionsDict) getCompletedRegistration(sessionID string) (registerResponse, bool) {
	d.RLock()
	defer d.RUnlock()
	result, ok := d.sessionCompletedResult[sessionID]
	return result, ok
}

func (d *sessionsDict) getDeviceToDelete(sessionID string) (string, bool) {
	d.RLock()
	defer d.RUnlock()
	deviceID, ok := d.deleteSessionToDeviceID[sessionID]
	return deviceID, ok
}

var (
	sessions           = newSessionsDict()
	validUsernameRegex = regexp.MustCompile(`^[0-9a-z_\-=./]+$`)
)

// registerRequest represents the submitted registration request.
// It can be broken down into 2 sections: the auth dictionary and registration parameters.
// Registration parameters vary depending on the request, and will need to remembered across
// sessions. If no parameters are supplied, the server should use the parameters previously
// remembered. If ANY parameters are supplied, the server should REPLACE all knowledge of
// previous parameters with the ones supplied. This mean you cannot "build up" request params.
type registerRequest struct {
	// registration parameters
	Password   string                       `json:"password"`
	Username   string                       `json:"username"`
	ServerName gomatrixserverlib.ServerName `json:"-"`
	Admin      bool                         `json:"admin"`
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

// newUserInteractiveResponse will return a struct to be sent back to the client
// during registration.
func newUserInteractiveResponse(
	sessionID string,
	fs []authtypes.Flow,
	params map[string]interface{},
) userInteractiveResponse {
	return userInteractiveResponse{
		fs, sessions.getCompletedStages(sessionID), params, sessionID,
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
func validateUsername(localpart string, domain gomatrixserverlib.ServerName) *util.JSONResponse {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if id := fmt.Sprintf("@%s:%s", localpart, domain); len(id) > maxUsernameLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(fmt.Sprintf("%q exceeds the maximum length of %d characters", id, maxUsernameLength)),
		}
	} else if !validUsernameRegex.MatchString(localpart) {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Username can only contain characters a-z, 0-9, or '_-./='"),
		}
	} else if localpart[0] == '_' { // Regex checks its not a zero length string
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Username cannot start with a '_'"),
		}
	}
	return nil
}

// validateApplicationServiceUsername returns an error response if the username is invalid for an application service
func validateApplicationServiceUsername(localpart string, domain gomatrixserverlib.ServerName) *util.JSONResponse {
	if id := fmt.Sprintf("@%s:%s", localpart, domain); len(id) > maxUsernameLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(fmt.Sprintf("%q exceeds the maximum length of %d characters", id, maxUsernameLength)),
		}
	} else if !validUsernameRegex.MatchString(localpart) {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidUsername("Username can only contain characters a-z, 0-9, or '_-./='"),
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
	ip, _, _ := net.SplitHostPort(clientip)
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
			"remoteip": {ip},
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
	body, err := io.ReadAll(resp.Body)
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

	var local, domain, err = gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		// Not a valid userID
		return false
	}

	if !cfg.Matrix.IsLocalServerName(domain) {
		return false
	}

	if appservice != nil {
		if appservice.SenderLocalpart == local {
			return true
		}

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
		if knownAppService.SenderLocalpart == local {
			return true
		}
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
	if err := validateApplicationServiceUsername(username, cfg.Matrix.ServerName); err != nil {
		return "", err
	}

	// No errors, registration valid
	return matchedApplicationService.ID, nil
}

// Register processes a /register request.
// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
func Register(
	req *http.Request,
	userAPI userapi.ClientUserAPI,
	cfg *config.ClientAPI,
) util.JSONResponse {
	defer req.Body.Close() // nolint: errcheck
	reqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("Unable to read request body"),
		}
	}

	var r registerRequest
	host := gomatrixserverlib.ServerName(req.Host)
	if v := cfg.Matrix.VirtualHostForHTTPHost(host); v != nil {
		r.ServerName = v.ServerName
	} else {
		r.ServerName = cfg.Matrix.ServerName
	}
	sessionID := gjson.GetBytes(reqBody, "auth.session").String()
	if sessionID == "" {
		// Generate a new, random session ID
		sessionID = util.RandomString(sessionIDLength)
	} else if data, ok := sessions.getParams(sessionID); ok {
		// Use the parameters from the session as our defaults.
		// Some of these might end up being overwritten if the
		// values are specified again in the request body.
		r.Username = data.Username
		r.ServerName = data.ServerName
		r.Password = data.Password
		r.DeviceID = data.DeviceID
		r.InitialDisplayName = data.InitialDisplayName
		r.InhibitLogin = data.InhibitLogin
		// Check if the user already registered using this session, if so, return that result
		if response, ok := sessions.getCompletedRegistration(sessionID); ok {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: response,
			}
		}
	}
	if resErr := httputil.UnmarshalJSON(reqBody, &r); resErr != nil {
		return *resErr
	}
	if l, d, err := cfg.Matrix.SplitLocalID('@', r.Username); err == nil {
		r.Username, r.ServerName = l, d
	}
	if req.URL.Query().Get("kind") == "guest" {
		return handleGuestRegistration(req, r, cfg, userAPI)
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
		nreq := &userapi.QueryNumericLocalpartRequest{
			ServerName: r.ServerName,
		}
		nres := &userapi.QueryNumericLocalpartResponse{}
		if err := userAPI.QueryNumericLocalpart(req.Context(), nreq, nres); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("userAPI.QueryNumericLocalpart failed")
			return jsonerror.InternalServerError()
		}
		r.Username = strconv.FormatInt(nres.ID, 10)
	}

	// Is this an appservice registration? It will be if the access
	// token is supplied
	accessToken, accessTokenErr := auth.ExtractAccessToken(req)

	// Squash username to all lowercase letters
	r.Username = strings.ToLower(r.Username)
	switch {
	case r.Type == authtypes.LoginTypeApplicationService && accessTokenErr == nil:
		// Spec-compliant case (the access_token is specified and the login type
		// is correctly set, so it's an appservice registration)
		if resErr := validateApplicationServiceUsername(r.Username, r.ServerName); resErr != nil {
			return *resErr
		}
	case accessTokenErr == nil:
		// Non-spec-compliant case (the access_token is specified but the login
		// type is not known or specified)
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("A known registration type (e.g. m.login.application_service) must be specified if an access_token is provided"),
		}
	default:
		// Spec-compliant case (neither the access_token nor the login type are
		// specified, so it's a normal user registration)
		if resErr := validateUsername(r.Username, r.ServerName); resErr != nil {
			return *resErr
		}
	}
	if resErr := validatePassword(r.Password); resErr != nil {
		return *resErr
	}

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"username":   r.Username,
		"auth.type":  r.Auth.Type,
		"session_id": r.Auth.Session,
	}).Info("Processing registration request")

	return handleRegistrationFlow(req, r, sessionID, cfg, userAPI, accessToken, accessTokenErr)
}

func handleGuestRegistration(
	req *http.Request,
	r registerRequest,
	cfg *config.ClientAPI,
	userAPI userapi.ClientUserAPI,
) util.JSONResponse {
	registrationEnabled := !cfg.RegistrationDisabled
	guestsEnabled := !cfg.GuestsDisabled
	if v := cfg.Matrix.VirtualHost(r.ServerName); v != nil {
		registrationEnabled, guestsEnabled = v.RegistrationAllowed()
	}

	if !registrationEnabled || !guestsEnabled {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(
				fmt.Sprintf("Guest registration is disabled on %q", r.ServerName),
			),
		}
	}

	var res userapi.PerformAccountCreationResponse
	err := userAPI.PerformAccountCreation(req.Context(), &userapi.PerformAccountCreationRequest{
		AccountType: userapi.AccountTypeGuest,
		ServerName:  r.ServerName,
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
		ServerName:        res.Account.ServerName,
		DeviceDisplayName: r.InitialDisplayName,
		AccessToken:       token,
		IPAddr:            req.RemoteAddr,
		UserAgent:         req.UserAgent(),
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
	userAPI userapi.ClientUserAPI,
	accessToken string,
	accessTokenErr error,
) util.JSONResponse {
	// TODO: Enable registration config flag
	// TODO: Guest account upgrading

	// TODO: Handle loading of previous session parameters from database.
	// TODO: Handle mapping registrationRequest parameters into session parameters

	// TODO: email / msisdn auth types.

	// Appservices are special and are not affected by disabled
	// registration or user exclusivity. We'll go onto the appservice
	// registration flow if a valid access token was provided or if
	// the login type specifically requests it.
	if r.Type == authtypes.LoginTypeApplicationService && accessTokenErr == nil {
		return handleApplicationServiceRegistration(
			accessToken, accessTokenErr, req, r, cfg, userAPI,
		)
	}

	registrationEnabled := !cfg.RegistrationDisabled
	if v := cfg.Matrix.VirtualHost(r.ServerName); v != nil {
		registrationEnabled, _ = v.RegistrationAllowed()
	}
	if !registrationEnabled && r.Auth.Type != authtypes.LoginTypeSharedSecret {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(
				fmt.Sprintf("Registration is disabled on %q", r.ServerName),
			),
		}
	}

	// Make sure normal user isn't registering under an exclusive application
	// service namespace. Skip this check if no app services are registered.
	// If an access token is provided, ignore this check this is an appservice
	// request and we will validate in validateApplicationService
	if len(cfg.Derived.ApplicationServices) != 0 &&
		UsernameMatchesExclusiveNamespaces(cfg, r.Username) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.ASExclusive("This username is reserved by an application service."),
		}
	}

	switch r.Auth.Type {
	case authtypes.LoginTypeRecaptcha:
		// Check given captcha response
		resErr := validateRecaptcha(cfg, r.Auth.Response, req.RemoteAddr)
		if resErr != nil {
			return *resErr
		}

		// Add Recaptcha to the list of completed registration stages
		sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypeRecaptcha)

	case authtypes.LoginTypeDummy:
		// there is nothing to do
		// Add Dummy to the list of completed registration stages
		sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypeDummy)

	case "":
		// An empty auth type means that we want to fetch the available
		// flows. It can also mean that we want to register as an appservice
		// but that is handed above.
	default:
		return util.JSONResponse{
			Code: http.StatusNotImplemented,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}

	// Check if the user's registration flow has been completed successfully
	// A response with current registration flow and remaining available methods
	// will be returned if a flow has not been successfully completed yet
	return checkAndCompleteFlow(sessions.getCompletedStages(sessionID),
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
	userAPI userapi.ClientUserAPI,
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
		req.Context(), userAPI, r.Username, r.ServerName, "", appserviceID, req.RemoteAddr,
		req.UserAgent(), r.Auth.Session, r.InhibitLogin, r.InitialDisplayName, r.DeviceID,
		userapi.AccountTypeAppService,
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
	userAPI userapi.ClientUserAPI,
) util.JSONResponse {
	if checkFlowCompleted(flow, cfg.Derived.Registration.Flows) {
		// This flow was completed, registration can continue
		return completeRegistration(
			req.Context(), userAPI, r.Username, r.ServerName, r.Password, "", req.RemoteAddr,
			req.UserAgent(), sessionID, r.InhibitLogin, r.InitialDisplayName, r.DeviceID,
			userapi.AccountTypeUser,
		)
	}
	sessions.addParams(sessionID, r)
	// There are still more stages to complete.
	// Return the flows and those that have been completed.
	return util.JSONResponse{
		Code: http.StatusUnauthorized,
		JSON: newUserInteractiveResponse(sessionID,
			cfg.Derived.Registration.Flows, cfg.Derived.Registration.Params),
	}
}

// completeRegistration runs some rudimentary checks against the submitted
// input, then if successful creates an account and a newly associated device
// We pass in each individual part of the request here instead of just passing a
// registerRequest, as this function serves requests encoded as both
// registerRequests and legacyRegisterRequests, which share some attributes but
// not all
func completeRegistration(
	ctx context.Context,
	userAPI userapi.ClientUserAPI,
	username string, serverName gomatrixserverlib.ServerName,
	password, appserviceID, ipAddr, userAgent, sessionID string,
	inhibitLogin eventutil.WeakBoolean,
	displayName, deviceID *string,
	accType userapi.AccountType,
) util.JSONResponse {
	if username == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Missing username"),
		}
	}
	// Blank passwords are only allowed by registered application services
	if password == "" && appserviceID == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Missing password"),
		}
	}
	var accRes userapi.PerformAccountCreationResponse
	err := userAPI.PerformAccountCreation(ctx, &userapi.PerformAccountCreationRequest{
		AppServiceID: appserviceID,
		Localpart:    username,
		ServerName:   serverName,
		Password:     password,
		AccountType:  accType,
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
		ServerName:        serverName,
		AccessToken:       token,
		DeviceDisplayName: displayName,
		DeviceID:          deviceID,
		IPAddr:            ipAddr,
		UserAgent:         userAgent,
	}, &devRes)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to create device: " + err.Error()),
		}
	}

	result := registerResponse{
		UserID:      devRes.Device.UserID,
		AccessToken: devRes.Device.AccessToken,
		HomeServer:  accRes.Account.ServerName,
		DeviceID:    devRes.Device.ID,
	}
	sessions.addCompletedRegistration(sessionID, result)

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: result,
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
	registerAPI userapi.ClientUserAPI,
) util.JSONResponse {
	username := req.URL.Query().Get("username")

	// Squash username to all lowercase letters
	username = strings.ToLower(username)
	domain := cfg.Matrix.ServerName
	host := gomatrixserverlib.ServerName(req.Host)
	if v := cfg.Matrix.VirtualHostForHTTPHost(host); v != nil {
		domain = v.ServerName
	}
	if u, l, err := cfg.Matrix.SplitLocalID('@', username); err == nil {
		username, domain = u, l
	}
	for _, v := range cfg.Matrix.VirtualHosts {
		if v.ServerName == domain && !v.AllowRegistration {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden(
					fmt.Sprintf("Registration is not allowed on %q", string(v.ServerName)),
				),
			}
		}
	}

	if err := validateUsername(username, domain); err != nil {
		return *err
	}

	// Check if this username is reserved by an application service
	userID := userutil.MakeUserID(username, domain)
	for _, appservice := range cfg.Derived.ApplicationServices {
		if appservice.OwnsNamespaceCoveringUserId(userID) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.UserInUse("Desired user ID is reserved by an application service."),
			}
		}
	}

	res := &userapi.QueryAccountAvailabilityResponse{}
	err := registerAPI.QueryAccountAvailability(req.Context(), &userapi.QueryAccountAvailabilityRequest{
		Localpart:  username,
		ServerName: domain,
	}, res)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("failed to check availability:" + err.Error()),
		}
	}

	if !res.Available {
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

func handleSharedSecretRegistration(cfg *config.ClientAPI, userAPI userapi.ClientUserAPI, sr *SharedSecretRegistration, req *http.Request) util.JSONResponse {
	ssrr, err := NewSharedSecretRegistrationRequest(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("malformed json: %s", err)),
		}
	}
	valid, err := sr.IsValidMacLogin(ssrr.Nonce, ssrr.User, ssrr.Password, ssrr.Admin, ssrr.MacBytes)
	if err != nil {
		return util.ErrorResponse(err)
	}
	if !valid {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("bad mac"),
		}
	}
	// downcase capitals
	ssrr.User = strings.ToLower(ssrr.User)

	if resErr := validateUsername(ssrr.User, cfg.Matrix.ServerName); resErr != nil {
		return *resErr
	}
	if resErr := validatePassword(ssrr.Password); resErr != nil {
		return *resErr
	}
	deviceID := "shared_secret_registration"

	accType := userapi.AccountTypeUser
	if ssrr.Admin {
		accType = userapi.AccountTypeAdmin
	}
	return completeRegistration(req.Context(), userAPI, ssrr.User, cfg.Matrix.ServerName, ssrr.Password, "", req.RemoteAddr, req.UserAgent(), "", false, &ssrr.User, &deviceID, accType)
}
