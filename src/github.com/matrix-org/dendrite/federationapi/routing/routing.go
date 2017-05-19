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
	"github.com/matrix-org/dendrite/federationapi/config"
	"github.com/matrix-org/dendrite/federationapi/readers"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
)

const (
	pathPrefixV2Keys = "/_matrix/keys/v2"
)

// Setup registers HTTP handlers with the given ServeMux.
func Setup(servMux *http.ServeMux, cfg config.FederationAPI) {
	apiMux := mux.NewRouter()
	v2keysmux := apiMux.PathPrefix(pathPrefixV2Keys).Subrouter()

	v2keysmux.Handle("/server/",
		makeAPI("localkeys", func(req *http.Request) util.JSONResponse {
			return readers.LocalKeys(req, cfg)
		}),
	)

	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

func makeAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	h := util.NewJSONRequestHandler(f)
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
