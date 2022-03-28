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

package httputil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth"
	federationapiAPI "github.com/matrix-org/dendrite/federationapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

// BasicAuth is used for authorization on /metrics handlers
type BasicAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// MakeAuthAPI turns a util.JSONRequestHandler function into an http.Handler which authenticates the request.
func MakeAuthAPI(
	metricsName string, userAPI userapi.UserInternalAPI,
	f func(*http.Request, *userapi.Device) util.JSONResponse,
) http.Handler {
	h := func(req *http.Request) util.JSONResponse {
		logger := util.GetLogger(req.Context())
		device, err := auth.VerifyUserFromRequest(req, userAPI)
		if err != nil {
			logger.Debugf("VerifyUserFromRequest %s -> HTTP %d", req.RemoteAddr, err.Code)
			return *err
		}
		// add the user ID to the logger
		logger = logger.WithField("user_id", device.UserID)
		req = req.WithContext(util.ContextWithLogger(req.Context(), logger))
		// add the user to Sentry, if enabled
		hub := sentry.GetHubFromContext(req.Context())
		if hub != nil {
			hub.Scope().SetTag("user_id", device.UserID)
			hub.Scope().SetTag("device_id", device.ID)
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

		jsonRes := f(req, device)
		// do not log 4xx as errors as they are client fails, not server fails
		if hub != nil && jsonRes.Code >= 500 {
			hub.Scope().SetExtra("response", jsonRes)
			hub.CaptureException(fmt.Errorf("%s returned HTTP %d", req.URL.Path, jsonRes.Code))
		}
		return jsonRes
	}
	return MakeExternalAPI(metricsName, h)
}

// MakeExternalAPI turns a util.JSONRequestHandler function into an http.Handler.
// This is used for APIs that are called from the internet.
func MakeExternalAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	// TODO: We shouldn't be directly reading env vars here, inject it in instead.
	// Refactor this when we split out config structs.
	verbose := false
	if os.Getenv("DENDRITE_TRACE_HTTP") == "1" {
		verbose = true
	}
	h := util.MakeJSONAPI(util.NewJSONRequestHandler(f))
	withSpan := func(w http.ResponseWriter, req *http.Request) {
		nextWriter := w
		if verbose {
			logger := logrus.NewEntry(logrus.StandardLogger())
			// Log outgoing response
			rec := httptest.NewRecorder()
			nextWriter = rec
			defer func() {
				resp := rec.Result()
				dump, err := httputil.DumpResponse(resp, true)
				if err != nil {
					logger.Debugf("Failed to dump outgoing response: %s", err)
				} else {
					strSlice := strings.Split(string(dump), "\n")
					for _, s := range strSlice {
						logger.Debug(s)
					}
				}
				// copy the response to the client
				for hdr, vals := range resp.Header {
					for _, val := range vals {
						w.Header().Add(hdr, val)
					}
				}
				w.WriteHeader(resp.StatusCode)
				// discard errors as this is for debugging
				_, _ = io.Copy(w, resp.Body)
				_ = resp.Body.Close()
			}()

			// Log incoming request
			dump, err := httputil.DumpRequest(req, true)
			if err != nil {
				logger.Debugf("Failed to dump incoming request: %s", err)
			} else {
				strSlice := strings.Split(string(dump), "\n")
				for _, s := range strSlice {
					logger.Debug(s)
				}
			}
		}

		span := opentracing.StartSpan(metricsName)
		defer span.Finish()
		req = req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
		h.ServeHTTP(nextWriter, req)

	}

	return http.HandlerFunc(withSpan)
}

// MakeHTMLAPI adds Span metrics to the HTML Handler function
// This is used to serve HTML alongside JSON error messages
func MakeHTMLAPI(metricsName string, f func(http.ResponseWriter, *http.Request) *util.JSONResponse) http.Handler {
	withSpan := func(w http.ResponseWriter, req *http.Request) {
		span := opentracing.StartSpan(metricsName)
		defer span.Finish()
		req = req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
		if err := f(w, req); err != nil {
			h := util.MakeJSONAPI(util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
				return *err
			}))
			h.ServeHTTP(w, req)
		}
	}

	return promhttp.InstrumentHandlerCounter(
		promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:      metricsName,
				Help:      "Total number of http requests for HTML resources",
				Namespace: "dendrite",
			},
			[]string{"code"},
		),
		http.HandlerFunc(withSpan),
	)
}

// MakeInternalAPI turns a util.JSONRequestHandler function into an http.Handler.
// This is used for APIs that are internal to dendrite.
// If we are passed a tracing context in the request headers then we use that
// as the parent of any tracing spans we create.
func MakeInternalAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	h := util.MakeJSONAPI(util.NewJSONRequestHandler(f))
	withSpan := func(w http.ResponseWriter, req *http.Request) {
		carrier := opentracing.HTTPHeadersCarrier(req.Header)
		tracer := opentracing.GlobalTracer()
		clientContext, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
		var span opentracing.Span
		if err == nil {
			// Default to a span without RPC context.
			span = tracer.StartSpan(metricsName)
		} else {
			// Set the RPC context.
			span = tracer.StartSpan(metricsName, ext.RPCServerOption(clientContext))
		}
		defer span.Finish()
		req = req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
		h.ServeHTTP(w, req)
	}

	return promhttp.InstrumentHandlerCounter(
		promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name:      metricsName + "_requests_total",
				Help:      "Total number of internal API calls",
				Namespace: "dendrite",
			},
			[]string{"code"},
		),
		promhttp.InstrumentHandlerResponseSize(
			promauto.NewHistogramVec(
				prometheus.HistogramOpts{
					Namespace: "dendrite",
					Name:      metricsName + "_response_size_bytes",
					Help:      "A histogram of response sizes for requests.",
					Buckets:   []float64{200, 500, 900, 1500, 5000, 15000, 50000, 100000},
				},
				[]string{},
			),
			http.HandlerFunc(withSpan),
		),
	)
}

// MakeFedAPI makes an http.Handler that checks matrix federation authentication.
func MakeFedAPI(
	metricsName string,
	serverName gomatrixserverlib.ServerName,
	keyRing gomatrixserverlib.JSONVerifier,
	wakeup *FederationWakeups,
	f func(*http.Request, *gomatrixserverlib.FederationRequest, map[string]string) util.JSONResponse,
) http.Handler {
	h := func(req *http.Request) util.JSONResponse {
		fedReq, errResp := gomatrixserverlib.VerifyHTTPRequest(
			req, time.Now(), serverName, keyRing,
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
		go wakeup.Wakeup(req.Context(), fedReq.Origin())
		vars, err := URLDecodeMapValues(mux.Vars(req))
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
	return MakeExternalAPI(metricsName, h)
}

type FederationWakeups struct {
	FsAPI   federationapiAPI.FederationInternalAPI
	origins sync.Map
}

func (f *FederationWakeups) Wakeup(ctx context.Context, origin gomatrixserverlib.ServerName) {
	key, keyok := f.origins.Load(origin)
	if keyok {
		lastTime, ok := key.(time.Time)
		if ok && time.Since(lastTime) < time.Minute {
			return
		}
	}
	aliveReq := federationapiAPI.PerformServersAliveRequest{
		Servers: []gomatrixserverlib.ServerName{origin},
	}
	aliveRes := federationapiAPI.PerformServersAliveResponse{}
	if err := f.FsAPI.PerformServersAlive(ctx, &aliveReq, &aliveRes); err != nil {
		util.GetLogger(ctx).WithError(err).WithFields(logrus.Fields{
			"origin": origin,
		}).Warn("incoming federation request failed to notify server alive")
	} else {
		f.origins.Store(origin, time.Now())
	}
}

// WrapHandlerInBasicAuth adds basic auth to a handler. Only used for /metrics
func WrapHandlerInBasicAuth(h http.Handler, b BasicAuth) http.HandlerFunc {
	if b.Username == "" || b.Password == "" {
		logrus.Warn("Metrics are exposed without protection. Make sure you set up protection at proxy level.")
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// Serve without authorization if either Username or Password is unset
		if b.Username == "" || b.Password == "" {
			h.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()

		if !ok || user != b.Username || pass != b.Password {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	}
}

// WrapHandlerInCORS adds CORS headers to all responses, including all error
// responses.
// Handles OPTIONS requests directly.
func WrapHandlerInCORS(h http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept, Authorization")

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			// Its easiest just to always return a 200 OK for everything. Whether
			// this is technically correct or not is a question, but in the end this
			// is what a lot of other people do (including synapse) and the clients
			// are perfectly happy with it.
			w.WriteHeader(http.StatusOK)
		} else {
			h.ServeHTTP(w, r)
		}
	})
}
