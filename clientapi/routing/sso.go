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

package routing

import (
	"encoding/xml"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/util"
)

// the XML response structure of CAS ticket validation
type casValidateResponse struct {
	XMLName               xml.Name `xml:"serviceResponse"`
	Cas                   string   `xml:"cas,attr"`
	AuthenticationSuccess struct {
		User string `xml:"user"`
	} `xml:"authenticationSuccess"`
}

// SSORedirect implements GET /login/sso/redirect
// https://matrix.org/docs/spec/client_server/r0.6.1#get-matrix-client-r0-login-sso-redirect
// If the incoming request doesn't contain a SSO token, it will redirect to the SSO server
// Else it will validate the SSO token, and redirect to the "redirectURL" provided with an extra "loginToken" param
func SSORedirect(
	req *http.Request,
	accountDB accounts.Database,
	cfg *config.ClientAPI,
) util.JSONResponse {
	// If dendrite is not configured to use SSO by the admin return bad method
	if !cfg.CAS.Enabled || cfg.CAS.Server == "" {
		return util.JSONResponse{
			Code: http.StatusNotImplemented,
			JSON: jsonerror.NotFound("Method disabled"),
		}
	}

	// A redirect URL is required for this endpoint
	redirectURLStr := req.FormValue("redirectUrl")
	if redirectURLStr == "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("redirectUrl parameter missing"),
		}
	}
	// Check if the redirect url is a valid URL
	redirectURL, err := url.Parse(redirectURLStr)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("Invalid redirectURL: " + err.Error()),
		}
	}

	// If the request has a ticket param, validate the ticket instead of redirecting to SSO server
	if ticket := req.FormValue("ticket"); ticket != "" {
		return ssoTicket(req, redirectURL, accountDB, cfg)
	}

	// Adding the params to the sso url
	ssoURL := cfg.CAS.URL
	ssoQueries := make(url.Values)
	// the service url that we send to CAS is homeserver.com/_matrix/client/r0/login/sso/redirect?redirectUrl=xyz
	ssoQueries.Set("service", req.RequestURI)
	ssoURL.RawQuery = ssoQueries.Encode()

	return util.RedirectResponse(ssoURL.String())
}

// ssoTicket handles the m.login.sso login attempt after the user had completed auth at the SSO server
// - gets the ticket from the SSO server (this is different from the matrix login/access token)
// - calls validateTicket to validate the ticket
// - calls completeSSOAuth
func ssoTicket(
	req *http.Request,
	redirectURL *url.URL,
	accountDB accounts.Database,
	cfg *config.ClientAPI,
) util.JSONResponse {
	// form the ticket validation URL from the config
	validateURL := cfg.CAS.ValidateURL
	ticket := req.FormValue("ticket")

	// append required params to the CAS validate endpoint
	validateQueries := make(url.Values)
	validateQueries.Set("ticket", ticket)
	validateURL.RawQuery = validateQueries.Encode()

	// validate the ticket
	casUsername, err := validateTicket(validateURL.String())
	if err != nil {
		// TODO: should I be logging these? What else should I log?
		util.GetLogger(req.Context()).WithError(err).Error("CAS SSO ticket validation failed")
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.Unknown("Could not validate SSO token: " + err.Error()),
		}
	}
	if casUsername == "" {
		util.GetLogger(req.Context()).WithError(err).Error("CAS SSO returned no user")
		return util.JSONResponse{
			Code: http.StatusUnauthorized,
			JSON: jsonerror.Unknown("CAS SSO returned no user"),
		}
	}

	// ticket validated. Login the user
	return completeSSOAuth(req, casUsername, redirectURL, accountDB)
}

// validateTicket sends the ticket to the sso server to get it validated
// the CAS server responds with an xml which contains the username
// validateTicket returns the SSO User
func validateTicket(
	ssoURL string,
) (string, error) {
	// make the call to the sso server to validate
	response, err := http.Get(ssoURL)
	if err != nil {
		return "", err
	}

	// extract the response from the sso server
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	// parse the response to the CAS XML format
	var res casValidateResponse
	if err := xml.Unmarshal([]byte(data), &res); err != nil {
		return "", err
	}

	return res.AuthenticationSuccess.User, nil
}

// completeSSOAuth completes the SSO auth and returns a m.login.token for the client to authenticate with
// if the user doesn't exist, a new user is created
func completeSSOAuth(
	req *http.Request,
	username string,
	redirectURL *url.URL,
	accountDB accounts.Database,
) util.JSONResponse {
	// try to create an account with that username
	// if the user exists, then we pick that user, else we create a new user
	account, err := accountDB.CreateAccount(req.Context(), username, "", "")
	if err != nil {
		if err != sqlutil.ErrUserExists {
			// some error
			util.GetLogger(req.Context()).WithError(err).Error("Could not create new user")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: jsonerror.Unknown("Could not create new user"),
			}
		} else {
			// user already exists, so just pick up their details
			account, err = accountDB.GetAccountByLocalpart(req.Context(), username)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: jsonerror.Unknown("Could not query user"),
				}
			}
		}
	}
	token, err := auth.GenerateLoginToken(account.UserID)
	if err != nil || token == "" {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.Unknown("Could not generate login token"),
		}
	}

	// add the params to the sso url
	redirectQueries := make(url.Values)
	// the service url that we send to CAS is homeserver.com/_matrix/client/r0/login/sso/redirect?redirectUrl=xyz
	redirectQueries.Set("loginToken", token)

	redirectURL.RawQuery = redirectQueries.Encode()

	return util.RedirectResponse(redirectURL.String())
}
