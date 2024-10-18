// Copyright 2024 New Vector Ltd.
// Copyright 2019 Parminder Singh <parmsingh129@gmail.com>
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/element-hq/dendrite/clientapi/auth/authtypes"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/util"
)

// recaptchaTemplate is an HTML webpage template for recaptcha auth
const recaptchaTemplate = `
<html>
<head>
<title>Authentication</title>
<meta name='viewport' content='width=device-width, initial-scale=1,
    user-scalable=no, minimum-scale=1.0, maximum-scale=1.0'>
<script src="{{.apiJsUrl}}" async defer></script>
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
        <div class="{{.sitekeyClass}}"
            data-sitekey="{{.sitekey}}"
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
) {
	// We currently only support "m.login.recaptcha", so fail early if that's not requested
	if authType == authtypes.LoginTypeRecaptcha {
		if !cfg.RecaptchaEnabled {
			writeHTTPMessage(w, req,
				"Recaptcha login is disabled on this Homeserver",
				http.StatusBadRequest,
			)
			return
		}
	} else {
		writeHTTPMessage(w, req, fmt.Sprintf("Unknown authtype %q", authType), http.StatusNotImplemented)
		return
	}

	sessionID := req.URL.Query().Get("session")
	if sessionID == "" {
		writeHTTPMessage(w, req,
			"Session ID not provided",
			http.StatusBadRequest,
		)
		return
	}

	serveRecaptcha := func() {
		data := map[string]string{
			"myUrl":        req.URL.String(),
			"session":      sessionID,
			"apiJsUrl":     cfg.RecaptchaApiJsUrl,
			"sitekey":      cfg.RecaptchaPublicKey,
			"sitekeyClass": cfg.RecaptchaSitekeyClass,
			"formField":    cfg.RecaptchaFormField,
		}
		serveTemplate(w, recaptchaTemplate, data)
	}

	serveSuccess := func() {
		data := map[string]string{}
		serveTemplate(w, successTemplate, data)
	}

	if req.Method == http.MethodGet {
		// Handle Recaptcha
		serveRecaptcha()
		return
	} else if req.Method == http.MethodPost {
		// Handle Recaptcha
		clientIP := req.RemoteAddr
		err := req.ParseForm()
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("req.ParseForm failed")
			w.WriteHeader(http.StatusBadRequest)
			serveRecaptcha()
			return
		}

		response := req.Form.Get(cfg.RecaptchaFormField)
		err = validateRecaptcha(cfg, response, clientIP)
		switch err {
		case ErrMissingResponse:
			w.WriteHeader(http.StatusBadRequest)
			serveRecaptcha() // serve the initial page again, instead of nothing
			return
		case ErrInvalidCaptcha:
			w.WriteHeader(http.StatusUnauthorized)
			serveRecaptcha()
			return
		case nil:
		default: // something else failed
			util.GetLogger(req.Context()).WithError(err).Error("failed to validate recaptcha")
			serveRecaptcha()
			return
		}

		// Success. Add recaptcha as a completed login flow
		sessions.addCompletedSessionStage(sessionID, authtypes.LoginTypeRecaptcha)

		serveSuccess()
		return
	}
	writeHTTPMessage(w, req, "Bad method", http.StatusMethodNotAllowed)
}

// writeHTTPMessage writes the given header and message to the HTTP response writer.
// Returns an error JSONResponse obtained through httputil.LogThenError if the writing failed, otherwise nil.
func writeHTTPMessage(
	w http.ResponseWriter, req *http.Request,
	message string, header int,
) {
	w.WriteHeader(header)
	_, err := w.Write([]byte(message))
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("w.Write failed")
	}
}
