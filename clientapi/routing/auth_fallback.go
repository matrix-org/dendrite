// Copyright 2019 Parminder Singh <parmsingh129@gmail.com>
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
	"html/template"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/util"
)

// recaptchaTemplate is an HTML webpage template for recaptcha auth
const recaptchaTemplate = `
<html>
<head>
<title>Authentication</title>
<meta name='viewport' content='width=device-width, initial-scale=1,
    user-scalable=no, minimum-scale=1.0, maximum-scale=1.0'>
<script src="https://www.google.com/recaptcha/api.js"
    async defer></script>
<script src="//code.jquery.com/jquery-1.11.2.min.js"></script>
<script>
function captchaDone() {
    $('#registrationForm').submit();
}
</script>
</head>
<body>
<form id="registrationForm" method="post" action="{{.myUrl}}">
    <div>
        <p>
        Hello! We need to prevent computer programs and other automated
        things from creating accounts on this server.
        </p>
        <p>
        Please verify that you're not a robot.
        </p>
		<input type="hidden" name="session" value="{{.session}}" />
        <div class="g-recaptcha"
            data-sitekey="{{.siteKey}}"
            data-callback="captchaDone">
        </div>
        <noscript>
        <input type="submit" value="All Done" />
        </noscript>
        </div>
    </div>
</form>
</body>
</html>
`

// successTemplate is an HTML template presented to the user after successful
// recaptcha completion
const successTemplate = `
<html>
<head>
<title>Success!</title>
<meta name='viewport' content='width=device-width, initial-scale=1,
    user-scalable=no, minimum-scale=1.0, maximum-scale=1.0'>
<script>
if (window.onAuthDone) {
    window.onAuthDone();
} else if (window.opener && window.opener.postMessage) {
    window.opener.postMessage("authDone", "*");
}
</script>
</head>
<body>
    <div>
        <p>Thank you!</p>
        <p>You may now close this window and return to the application.</p>
    </div>
</body>
</html>
`

// serveTemplate fills template data and serves it using http.ResponseWriter
func serveTemplate(w http.ResponseWriter, templateHTML string, data map[string]string) {
	t := template.Must(template.New("response").Parse(templateHTML))
	if err := t.Execute(w, data); err != nil {
		panic(err)
	}
}

// AuthFallback implements GET and POST /auth/{authType}/fallback/web?session={sessionID}
func AuthFallback(
	w http.ResponseWriter, req *http.Request, authType string,
	cfg *config.ClientAPI,
) *util.JSONResponse {
	sessionID := req.URL.Query().Get("session")

	if sessionID == "" {
		return writeHTTPMessage(w, req,
			"Session ID not provided",
			http.StatusBadRequest,
		)
	}

	serveRecaptcha := func() {
		data := map[string]string{
			"myUrl":   req.URL.String(),
			"session": sessionID,
			"siteKey": cfg.RecaptchaPublicKey,
		}
		serveTemplate(w, recaptchaTemplate, data)
	}

	serveSuccess := func() {
		data := map[string]string{}
		serveTemplate(w, successTemplate, data)
	}

	if req.Method == http.MethodGet {
		// Handle Recaptcha
		if authType == authtypes.LoginTypeRecaptcha {
			if err := checkRecaptchaEnabled(cfg, w, req); err != nil {
				return err
			}

			serveRecaptcha()
			return nil
		}
		return &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Unknown auth stage type"),
		}
	} else if req.Method == http.MethodPost {
		// Handle Recaptcha
		if authType == authtypes.LoginTypeRecaptcha {
			if err := checkRecaptchaEnabled(cfg, w, req); err != nil {
				return err
			}

			clientIP := req.RemoteAddr
			err := req.ParseForm()
			if err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("req.ParseForm failed")
				res := jsonerror.InternalServerError()
				return &res
			}

			response := req.Form.Get("g-recaptcha-response")
			if err := validateRecaptcha(cfg, response, clientIP); err != nil {
				util.GetLogger(req.Context()).Error(err)
				return err
			}

			// Success. Add recaptcha as a completed login flow
			sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypeRecaptcha)

			serveSuccess()
			return nil
		}

		return &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Unknown auth stage type"),
		}
	}
	return &util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

// checkRecaptchaEnabled creates an error response if recaptcha is not usable on homeserver.
func checkRecaptchaEnabled(
	cfg *config.ClientAPI,
	w http.ResponseWriter,
	req *http.Request,
) *util.JSONResponse {
	if !cfg.RecaptchaEnabled {
		return writeHTTPMessage(w, req,
			"Recaptcha login is disabled on this Homeserver",
			http.StatusBadRequest,
		)
	}
	return nil
}

// writeHTTPMessage writes the given header and message to the HTTP response writer.
// Returns an error JSONResponse obtained through httputil.LogThenError if the writing failed, otherwise nil.
func writeHTTPMessage(
	w http.ResponseWriter, req *http.Request,
	message string, header int,
) *util.JSONResponse {
	w.WriteHeader(header)
	_, err := w.Write([]byte(message))
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("w.Write failed")
		res := jsonerror.InternalServerError()
		return &res
	}
	return nil
}
