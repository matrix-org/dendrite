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
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/readers"
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
	r0mux.Handle("/createRoom",
		makeAPI("createRoom", func(req *http.Request) util.JSONResponse {
			return writers.CreateRoom(req, cfg, producer)
		}),
	)
	r0mux.Handle("/rooms/{roomID}/send/{eventType}/{txnID}",
		makeAPI("send_message", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.SendEvent(req, vars["roomID"], vars["eventType"], vars["txnID"], nil, cfg, queryAPI, producer)
		}),
	)
	r0mux.Handle("/rooms/{roomID}/state/{eventType}",
		makeAPI("send_message", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			emptyString := ""
			return writers.SendEvent(req, vars["roomID"], vars["eventType"], vars["txnID"], &emptyString, cfg, queryAPI, producer)
		}),
	)
	r0mux.Handle("/rooms/{roomID}/state/{eventType}/{stateKey}",
		makeAPI("send_message", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			stateKey := vars["stateKey"]
			return writers.SendEvent(req, vars["roomID"], vars["eventType"], vars["txnID"], &stateKey, cfg, queryAPI, producer)
		}),
	)

	// Stub endpoints required by Riot

	r0mux.Handle("/login",
		makeAPI("login", func(req *http.Request) util.JSONResponse {
			return readers.Login(req, cfg)
		}),
	)

	r0mux.Handle("/pushrules/",
		makeAPI("push_rules", func(req *http.Request) util.JSONResponse {
			// TODO: Implement push rules API
			res := json.RawMessage(`{
					"global": {
						"content": [],
						"override": [],
						"room": [],
						"sender": [],
						"underride": []
					}
				}`)
			return util.JSONResponse{
				Code: 200,
				JSON: &res,
			}
		}),
	)

	r0mux.Handle("/user/{userID}/filter",
		makeAPI("make_filter", func(req *http.Request) util.JSONResponse {
			// TODO: Persist filter and return filter ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	)

	r0mux.Handle("/user/{userID}/filter/{filterID}",
		makeAPI("filter", func(req *http.Request) util.JSONResponse {
			// TODO: Retrieve filter based on ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	)

	// Riot user settings

	r0mux.Handle("/profile/{userID}",
		makeAPI("profile", func(req *http.Request) util.JSONResponse {
			// TODO: Get profile data for user ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	)

	r0mux.Handle("/account/3pid",
		makeAPI("account_3pid", func(req *http.Request) util.JSONResponse {
			// TODO: Get 3pid data for user ID
			res := json.RawMessage(`{"threepids":[]}`)
			return util.JSONResponse{
				Code: 200,
				JSON: &res,
			}
		}),
	)

	// Riot logs get flooded unless this is handled
	r0mux.Handle("/presence/{userID}/status",
		makeAPI("presence", func(req *http.Request) util.JSONResponse {
			// TODO: Set presence (probably the responsibility of a presence server not clientapi)
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	)

	servMux.Handle("/metrics", prometheus.Handler())
	servMux.Handle("/api/", http.StripPrefix("/api", apiMux))
}

// make a util.JSONRequestHandler function into an http.Handler.
func makeAPI(metricsName string, f func(*http.Request) util.JSONResponse) http.Handler {
	h := util.NewJSONRequestHander(f)
	return prometheus.InstrumentHandler(metricsName, util.MakeJSONAPI(h))
}
