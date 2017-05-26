package writers

import (
	"fmt"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const (
	minPasswordLength = 8   // http://matrix.org/docs/spec/client_server/r0.2.0.html#password-based
	maxPasswordLength = 512 // https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	maxUsernameLength = 254 // http://matrix.org/speculator/spec/HEAD/intro.html#user-identifiers TODO account for domain
)

// registerRequest represents the submitted registration request.
// It can be broken down into 2 sections: the auth dictionary and registration parameters.
// Registration parameters vary depending on the request, and will need to remembered across
// sessions. If no parameters are supplied, the server should use the parameters previously
// remembered. If ANY parameters are supplied, the server should REPLACE all knowledge of
// previous paramters with the ones supplied. This mean you cannot "build up" request params.
type registerRequest struct {
	// registration parameters.
	Password string `json:"password"`
	Username string `json:"username"`
	// user-interactive auth params
	Auth authDict `json:"auth"`
}

type authDict struct {
	Type    authtypes.LoginType `json:"type"`
	Session string              `json:"session"`
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

// Validate returns an error response if the request fails to validate.
func (r *registerRequest) Validate() *util.JSONResponse {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if len(r.Password) > maxPasswordLength {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'password' >%d characters", maxPasswordLength)),
		}
	} else if len(r.Username) > maxUsernameLength {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON(fmt.Sprintf("'username' >%d characters", maxUsernameLength)),
		}
	} else if len(r.Password) > 0 && len(r.Password) < minPasswordLength {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.WeakPassword(fmt.Sprintf("password too weak: min %d chars", minPasswordLength)),
		}
	}
	return nil
}

// Register processes a /register request. http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
func Register(req *http.Request, accountDB *accounts.Database, deviceDB *devices.Database) util.JSONResponse {
	var r registerRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}
	if resErr = r.Validate(); resErr != nil {
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

	// All registration requests must specify what auth they are using to perform this request
	if r.Auth.Type == "" {
		return util.JSONResponse{
			Code: 401,
			// TODO: Hard-coded 'dummy' auth for now with a bogus session ID.
			//       Server admins should be able to change things around (eg enable captcha)
			JSON: newUserInteractiveResponse("totallyuniquesessionid", []authFlow{
				{[]authtypes.LoginType{authtypes.LoginTypeDummy}},
			}),
		}
	}

	// TODO: Handle loading of previous session parameters from database.
	// TODO: Handle mapping registrationRequest parameters into session parameters

	// TODO: email / msisdn / recaptcha auth types.
	switch r.Auth.Type {
	case authtypes.LoginTypeDummy:
		// there is nothing to do
		return completeRegistration(accountDB, deviceDB, r.Username, r.Password)
	default:
		return util.JSONResponse{
			Code: 501,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}
}

func completeRegistration(accountDB *accounts.Database, deviceDB *devices.Database, username, password string) util.JSONResponse {
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

	acc, err := accountDB.CreateAccount(username, password)
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
	dev, err := deviceDB.CreateDevice(username, auth.UnknownDeviceID, token)
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
