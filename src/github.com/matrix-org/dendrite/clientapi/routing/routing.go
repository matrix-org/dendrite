package routing

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/writers"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(servMux *http.ServeMux, httpClient *http.Client, cfg config.ClientAPI, producer *producers.RoomserverProducer, queryAPI api.RoomserverQueryAPI) {
	apiMux := mux.NewRouter()
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/createRoom", make("createRoom", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
		return writers.CreateRoom(req, cfg, producer)
	})))
	r0mux.Handle("/rooms/{roomID}/send/{eventType}/{txnID}",
		make("send_message", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.SendEvent(req, vars["roomID"], vars["eventType"], vars["txnID"], nil, cfg, queryAPI, producer)
		})),
	)
	r0mux.Handle("/rooms/{roomID}/state/{eventType}",
		make("send_message", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			emptyString := ""
			return writers.SendEvent(req, vars["roomID"], vars["eventType"], vars["txnID"], &emptyString, cfg, queryAPI, producer)
		})),
	)
	r0mux.Handle("/rooms/{roomID}/state/{eventType}/{stateKey}",
		make("send_message", util.NewJSONRequestHandler(func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			stateKey := vars["stateKey"]
			return writers.SendEvent(req, vars["roomID"], vars["eventType"], vars["txnID"], &stateKey, cfg, queryAPI, producer)
		})),
	)

	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

// make a util.JSONRequestHandler into an http.Handler
func make(metricsName string, h util.JSONRequestHandler) http.Handler {
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
