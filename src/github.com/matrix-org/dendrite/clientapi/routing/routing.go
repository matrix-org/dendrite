package routing

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/readers"
	"github.com/matrix-org/dendrite/clientapi/writers"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	sarama "gopkg.in/Shopify/sarama.v1"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(servMux *http.ServeMux, httpClient *http.Client, cfg config.ClientAPI, producer sarama.SyncProducer) {
	apiMux := mux.NewRouter()
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/createRoom", make("createRoom", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
		return writers.CreateRoom(req, cfg, producer)
	})))
	r0mux.Handle("/sync", make("sync", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
		return readers.Sync(req)
	})))
	r0mux.Handle("/rooms/{roomID}/send/{eventType}",
		make("send_message", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.SendMessage(req, vars["roomID"], vars["eventType"])
		})),
	)

	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

// make a util.JSONRequestHandler into an http.Handler
func make(metricsName string, h util.JSONRequestHandler) http.Handler {
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
