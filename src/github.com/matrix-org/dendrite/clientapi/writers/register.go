package writers

import (
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth/storage"
	"github.com/matrix-org/dendrite/clientapi/auth/types"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
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
	Type    types.LoginType `json:"type"`
	Session string          `json:"session"`
}

// http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#user-interactive-authentication-api
type userInteractiveResponse struct {
	Flows     []flow                 `json:"flows"`
	Completed []types.LoginType      `json:"completed"`
	Params    map[string]interface{} `json:"params"`
	Session   string                 `json:"session"`
}

type flow struct {
	Stages []types.LoginType `json:"stages"`
}

func newUserInteractiveResponse(sessionID string, fs []flow) userInteractiveResponse {
	return userInteractiveResponse{
		fs, []types.LoginType{}, make(map[string]interface{}), sessionID,
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
	if len(r.Password) > 512 {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("'password' >512 characters"),
		}
	} else if len(r.Username) > 512 {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.BadJSON("'username' >512 characters"),
		}
	} else if len(r.Password) > 0 && len(r.Password) < 8 {
		return &util.JSONResponse{
			Code: 400,
			JSON: jsonerror.WeakPassword("password too weak: min 8 chars"),
		}
	}
	return nil
}

// Register processes a /register request. http://matrix.org/speculator/spec/HEAD/client_server/unstable.html#post-matrix-client-unstable-register
func Register(req *http.Request, accountDB *storage.AccountDatabase) util.JSONResponse {
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
			// Hard-coded 'dummy' auth for now with a bogus session ID.
			JSON: newUserInteractiveResponse("totallyuniquesessionid", []flow{
				flow{[]types.LoginType{types.LoginTypeDummy}},
			}),
		}
	}

	// TODO: Handle loading of previous session parameters from database.
	// TODO: Handle mapping registrationRequest parameters into session parameters

	// TODO: email / msisdn / recaptcha auth types.
	switch r.Auth.Type {
	case types.LoginTypeDummy:
		// there is nothing to do
		return completeRegistration(accountDB, r.Username, r.Password)
	default:
		return util.JSONResponse{
			Code: 501,
			JSON: jsonerror.Unknown("unknown/unimplemented auth type"),
		}
	}
}

func completeRegistration(accountDB *storage.AccountDatabase, username, password string) util.JSONResponse {
	acc, err := accountDB.CreateAccount(username, password)
	if err != nil {
		return util.JSONResponse{
			Code: 500,
			JSON: jsonerror.Unknown("failed to create account: " + err.Error()),
		}
	}
	// TODO: Make and store a proper access_token
	// TODO: Store the client's device information?
	return util.JSONResponse{
		Code: 200,
		JSON: registerResponse{
			UserID:      acc.UserID,
			AccessToken: acc.UserID, // FIXME
			HomeServer:  acc.ServerName,
			DeviceID:    "dendrite",
		},
	}
}
