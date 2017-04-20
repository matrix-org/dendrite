package routing

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/syncapi/config"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

const pathPrefixR0 = "/_matrix/client/r0"

// SetupSyncServerListeners configures the given mux with sync-server listeners
func SetupSyncServerListeners(servMux *http.ServeMux, httpClient *http.Client, cfg config.Sync, srp *sync.RequestPool) {
	apiMux := mux.NewRouter()
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/sync", make("sync", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
		return srp.OnIncomingSyncRequest(req)
	})))
	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

// make a util.JSONRequestHandler into an http.Handler
func make(metricsName string, h util.JSONRequestHandler) http.Handler {
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
