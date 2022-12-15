// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/httputil"
	relayInternal "github.com/matrix-org/dendrite/relayapi/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// Setup registers HTTP handlers with the given ServeMux.
// The provided publicAPIMux MUST have `UseEncodedPath()` enabled or else routes will incorrectly
// path unescape twice (once from the router, once from MakeRelayAPI). We need to have this enabled
// so we can decode paths like foo/bar%2Fbaz as [foo, bar/baz] - by default it will decode to [foo, bar, baz]
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	fedMux *mux.Router,
	cfg *config.FederationAPI,
	relayAPI *relayInternal.RelayInternalAPI,
	keys gomatrixserverlib.JSONVerifier,
) {
	v1fedmux := fedMux.PathPrefix("/v1").Subrouter()

	v1fedmux.Handle("/forward_async/{txnID}/{userID}", MakeRelayAPI(
		"relay_forward_async", "", cfg.Matrix.IsLocalServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			userID, err := gomatrixserverlib.NewUserID(vars["userID"], false)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: jsonerror.InvalidUsername("Username was invalid"),
				}
			}
			return ForwardAsync(
				httpReq, request, relayAPI, gomatrixserverlib.TransactionID(vars["txnID"]),
				*userID,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/async_events/{userID}", MakeRelayAPI(
		"relay_async_events", "", cfg.Matrix.IsLocalServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			userID, err := gomatrixserverlib.NewUserID(vars["userID"], false)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: jsonerror.InvalidUsername("Username was invalid"),
				}
			}
			return GetAsyncEvents(httpReq, request, relayAPI, *userID)
		},
	)).Methods(http.MethodGet, http.MethodOptions)
}

// MakeRelayAPI makes an http.Handler that checks matrix relay authentication.
func MakeRelayAPI(
	metricsName string, serverName gomatrixserverlib.ServerName,
	isLocalServerName func(gomatrixserverlib.ServerName) bool,
	keyRing gomatrixserverlib.JSONVerifier,
	f func(*http.Request, *gomatrixserverlib.FederationRequest, map[string]string) util.JSONResponse,
) http.Handler {
	h := func(req *http.Request) util.JSONResponse {
		fedReq, errResp := gomatrixserverlib.VerifyHTTPRequest(
			req, time.Now(), serverName, isLocalServerName, keyRing,
		)
		if fedReq == nil {
			return errResp
		}
		// add the user to Sentry, if enabled
		hub := sentry.GetHubFromContext(req.Context())
		if hub != nil {
			hub.Scope().SetTag("origin", string(fedReq.Origin()))
			hub.Scope().SetTag("uri", fedReq.RequestURI())
		}
		defer func() {
			if r := recover(); r != nil {
				if hub != nil {
					hub.CaptureException(fmt.Errorf("%s panicked", req.URL.Path))
				}
				// re-panic to return the 500
				panic(r)
			}
		}()
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.MatrixErrorResponse(400, "M_UNRECOGNISED", "badly encoded query params")
		}

		jsonRes := f(req, fedReq, vars)
		// do not log 4xx as errors as they are client fails, not server fails
		if hub != nil && jsonRes.Code >= 500 {
			hub.Scope().SetExtra("response", jsonRes)
			hub.CaptureException(fmt.Errorf("%s returned HTTP %d", req.URL.Path, jsonRes.Code))
		}
		return jsonRes
	}
	return httputil.MakeExternalAPI(metricsName, h)
}
