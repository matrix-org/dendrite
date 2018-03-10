// Copyright 2017 Vector Creations Ltd
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
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/util"
)

// RecaptchaTemplate is template for recaptcha auth
const RecaptchaTemplate = `
<html>
<head>
<title>Authentication</title>
<meta name='viewport' content='width=device-width, initial-scale=1,
    user-scalable=no, minimum-scale=1.0, maximum-scale=1.0'>
<script src="https://www.google.com/recaptcha/api.js"
    async defer></script>
<script src="//code.jquery.com/jquery-1.11.2.min.js"></script>
<link rel="stylesheet" href="/_matrix/static/client/register/style.css">
<script>
function captchaDone() {
    $('#registrationForm').submit();
}
</script>
</head>
<body>
<form id="registrationForm" method="post" action="{{.MyUrl}}">
    <div>
        <p>
        Hello! We need to prevent computer programs and other automated
        things from creating accounts on this server.
        </p>
        <p>
        Please verify that you're not a robot.
        </p>
		<input type="hidden" name="session" value="{{.Session}}" />
        <div class="g-recaptcha"
            data-sitekey="{{.SiteKey}}"
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

// SuccessTemplate is template for success page after auth flow ends
const SuccessTemplate = `
<html>
<head>
<title>Success!</title>
<meta name='viewport' content='width=device-width, initial-scale=1,
    user-scalable=no, minimum-scale=1.0, maximum-scale=1.0'>
<link rel="stylesheet" href="/_matrix/static/client/register/style.css">
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
        <p>Thank you</p>
        <p>You may now close this window and return to the application</p>
    </div>
</body>
</html>
`

// ServeTemplate fills data in template and serves it in http.ResponseWriter
func ServeTemplate(w http.ResponseWriter, templateHTML string, data map[string]string) {
	t := template.Must(template.New("response").Parse(templateHTML))
	if err := t.Execute(w, data); err != nil {
		panic(err)
	}
}

// AuthFallback implements GET on /auth/{authType}/fallback/web?session={sessionID}
func AuthFallback(
	w http.ResponseWriter, req *http.Request, authType string,
	cfg config.Dendrite,
) *util.JSONResponse {
	sessionID := req.URL.Query().Get("session")

	if sessionID == "" {
		w.Write([]byte("Session ID not provided"))
		return nil
	}

	ServeRecaptcha := func() {
		data := map[string]string{
			"MyUrl":   req.URL.String(),
			"Session": sessionID,
			"SiteKey": cfg.Matrix.RecaptchaPrivateKey,
		}
		ServeTemplate(w, RecaptchaTemplate, data)
	}

	ServeSuccess := func() {
		data := map[string]string{}
		ServeTemplate(w, SuccessTemplate, data)
	}

	if req.Method == "GET" {
		// Handle Recaptcha
		if authType == authtypes.LoginTypeRecaptcha {
			if cfg.Matrix.RecaptchaPrivateKey == "" {
				w.Write([]byte("Homeserver doesn't have a recaptcha public key"))
				return nil
			}

			ServeRecaptcha()
			return nil
		}
		return &util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("Unknown auth stage type"),
		}
	} else if req.Method == "POST" {
		clientIP := req.RemoteAddr
		response := req.Form.Get("g-recaptcha-response")
		if err := validateRecaptcha(&cfg, response, clientIP); err != nil {
			ServeRecaptcha()
			return nil
		}

		ServeSuccess()
		return nil
	}
	return &util.JSONResponse{
		Code: 405,
		JSON: jsonerror.NotFound("Bad method"),
	}
}
