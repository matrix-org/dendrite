package common

import (
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common/config"
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
	metricsName string, data auth.Data,
	f func(*http.Request, *authtypes.Device) util.JSONResponse,
) http.Handler {
	h := func(req *http.Request) util.JSONResponse {
		device, err := auth.VerifyUserFromRequest(req, data)
		if err != nil {
			return *err
		}

		return f(req, device)
	}
	return MakeExternalAPI(metricsName, h)
}

// MakeExternalAPI turns a util.JSONRequestHandler function into an http.Handler.
// This is used for APIs that are called from the internet.
func MakeExternalAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	h := util.MakeJSONAPI(util.NewJSONRequestHandler(f))
	withSpan := func(w http.ResponseWriter, req *http.Request) {
		span := opentracing.StartSpan(metricsName)
		defer span.Finish()
		req = req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
		h.ServeHTTP(w, req)
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
				Name: metricsName,
				Help: "Total number of http requests for HTML resources",
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

	return http.HandlerFunc(withSpan)
}

// MakeFedAPI makes an http.Handler that checks matrix federation authentication.
func MakeFedAPI(
	metricsName string,
	serverName gomatrixserverlib.ServerName,
	keyRing gomatrixserverlib.KeyRing,
	f func(*http.Request, *gomatrixserverlib.FederationRequest) util.JSONResponse,
) http.Handler {
	h := func(req *http.Request) util.JSONResponse {
		fedReq, errResp := gomatrixserverlib.VerifyHTTPRequest(
			req, time.Now(), serverName, keyRing,
		)
		if fedReq == nil {
			return errResp
		}
		return f(req, fedReq)
	}
	return MakeExternalAPI(metricsName, h)
}

// SetupHTTPAPI registers an HTTP API mux under /api and sets up a metrics
// listener.
func SetupHTTPAPI(servMux *http.ServeMux, apiMux http.Handler, cfg *config.Dendrite) {
	if cfg.Metrics.Enabled {
		servMux.Handle("/metrics", WrapHandlerInBasicAuth(promhttp.Handler(), cfg.Metrics.BasicAuth))
	}
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

// WrapHandlerInBasicAuth adds basic auth to a handler. Only used for /metrics
func WrapHandlerInBasicAuth(h http.Handler, b BasicAuth) http.HandlerFunc {
	if b.Username == "" || b.Password == "" {
		logrus.Info("Metrics are exposed without protection. Make sure you set up protection at proxy level.")
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
