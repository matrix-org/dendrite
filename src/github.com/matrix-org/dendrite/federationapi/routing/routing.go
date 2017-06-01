// Copyright 2017 Vector Creations Ltd
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
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/federationapi/config"
	"github.com/matrix-org/dendrite/federationapi/readers"
	"github.com/matrix-org/dendrite/federationapi/writers"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"time"
)

const (
	pathPrefixV2Keys       = "/_matrix/key/v2"
	pathPrefixV1Federation = "/_matrix/federation/v1"
)

// Setup registers HTTP handlers with the given ServeMux.
func Setup(
	servMux *http.ServeMux,
	cfg config.FederationAPI,
	query api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
) {
	apiMux := mux.NewRouter()
	v2keysmux := apiMux.PathPrefix(pathPrefixV2Keys).Subrouter()
	v1fedmux := apiMux.PathPrefix(pathPrefixV1Federation).Subrouter()

	localKeys := makeAPI("localkeys", func(req *http.Request) util.JSONResponse {
		return readers.LocalKeys(req, cfg)
	})

	// Ignore the {keyID} argument as we only have a single server key so we always
	// return that key.
	// Even if we had more than one server key, we would probably still ignore the
	// {keyID} argument and always return a response containing all of the keys.
	v2keysmux.Handle("/server/{keyID}", localKeys)
	v2keysmux.Handle("/server/", localKeys)

	v1fedmux.Handle("/send/{txnID}/", makeAPI("send",
		func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.Send(
				req, gomatrixserverlib.TransactionID(vars["txnID"]),
				time.Now(),
				cfg, query, producer, keys,
			)
		},
	))

	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

func makeAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	h := util.NewJSONRequestHandler(f)
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
