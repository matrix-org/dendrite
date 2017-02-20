package routing

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/readers"
	"github.com/matrix-org/dendrite/clientapi/writers"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(servMux *http.ServeMux, httpClient *http.Client) {
	apiMux := mux.NewRouter()
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/sync", make("sync", &readers.Sync{}))
	r0mux.Handle("/rooms/{roomID}/send/{eventType}",
		make("send_message", wrap(func(req *http.Request) (interface{}, *util.HTTPError) {
			vars := mux.Vars(req)
			roomID := vars["roomID"]
			eventType := vars["eventType"]
			h := &writers.SendMessage{}
			return h.OnIncomingRequest(req, roomID, eventType)
		})),
	)

	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

// make a util.JSONRequestHandler into an http.Handler
func make(metricsName string, h util.JSONRequestHandler) http.Handler {
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}

// jsonRequestHandlerWrapper is a wrapper to allow in-line functions to conform to util.JSONRequestHandler
type jsonRequestHandlerWrapper struct {
	function func(req *http.Request) (interface{}, *util.HTTPError)
}

func (r *jsonRequestHandlerWrapper) OnIncomingRequest(req *http.Request) (interface{}, *util.HTTPError) {
	return r.function(req)
}
func wrap(f func(req *http.Request) (interface{}, *util.HTTPError)) *jsonRequestHandlerWrapper {
	return &jsonRequestHandlerWrapper{f}
}
