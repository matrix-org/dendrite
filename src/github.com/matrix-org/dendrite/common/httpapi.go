package common

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

// MakeAuthAPI turns a util.JSONRequestHandler function into an http.Handler which checks the access token in the request.
func MakeAuthAPI(metricsName string, deviceDB auth.DeviceDatabase, f func(*http.Request, *authtypes.Device) util.JSONResponse) http.Handler {
	h := func(req *http.Request) util.JSONResponse {
		device, resErr := auth.VerifyAccessToken(req, deviceDB)
		if resErr != nil {
			return *resErr
		}
		return f(req, device)
	}
	return MakeAPI(metricsName, h)
}

// MakeAPI turns a util.JSONRequestHandler function into an http.Handler.
func MakeAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	h := util.NewJSONRequestHandler(f)
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
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
	return MakeAPI(metricsName, h)
}

// SetupHTTPAPI registers an HTTP API mux under /api and sets up a metrics
// listener.
func SetupHTTPAPI(servMux *http.ServeMux, apiMux *mux.Router) {
	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}
