package common

import (
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

// MakeAuthAPI turns a util.JSONRequestHandler function into an http.Handler which checks the access token in the request.
func MakeAuthAPI(metricsName string, deviceDB *devices.Database, f func(*http.Request, *authtypes.Device) util.JSONResponse) http.Handler {
	h := util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
		device, resErr := auth.VerifyAccessToken(req, deviceDB)
		if resErr != nil {
			return *resErr
		}
		return f(req, device)
	})
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}

// MakeAPI turns a util.JSONRequestHandler function into an http.Handler.
func MakeAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	h := util.NewJSONRequestHandler(f)
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
