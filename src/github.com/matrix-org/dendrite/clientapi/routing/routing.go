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
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/readers"
	"github.com/matrix-org/dendrite/clientapi/writers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
)

const pathPrefixR0 = "/_matrix/client/r0"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(
	servMux *http.ServeMux, httpClient *http.Client, cfg config.ClientAPI,
	producer *producers.RoomserverProducer, queryAPI api.RoomserverQueryAPI,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
	federation *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.KeyRing,
) {
	apiMux := mux.NewRouter()
	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	r0mux.Handle("/createRoom",
		common.MakeAuthAPI("createRoom", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return writers.CreateRoom(req, device, cfg, producer)
		}),
	)
	r0mux.Handle("/join/{roomIDOrAlias}",
		common.MakeAuthAPI("join", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.JoinRoomByIDOrAlias(
				req, device, vars["roomIDOrAlias"], cfg, federation, producer, queryAPI, keyRing,
			)
		}),
	)
	r0mux.Handle("/rooms/{roomID}/send/{eventType}/{txnID}",
		common.MakeAuthAPI("send_message", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.SendEvent(req, device, vars["roomID"], vars["eventType"], vars["txnID"], nil, cfg, queryAPI, producer)
		}),
	)
	r0mux.Handle("/rooms/{roomID}/state/{eventType}",
		common.MakeAuthAPI("send_message", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			emptyString := ""
			return writers.SendEvent(req, device, vars["roomID"], vars["eventType"], vars["txnID"], &emptyString, cfg, queryAPI, producer)
		}),
	)
	r0mux.Handle("/rooms/{roomID}/state/{eventType}/{stateKey}",
		common.MakeAuthAPI("send_message", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			stateKey := vars["stateKey"]
			return writers.SendEvent(req, device, vars["roomID"], vars["eventType"], vars["txnID"], &stateKey, cfg, queryAPI, producer)
		}),
	)

	r0mux.Handle("/register", common.MakeAPI("register", func(req *http.Request) util.JSONResponse {
		return writers.Register(req, accountDB, deviceDB)
	}))

	// Stub endpoints required by Riot

	r0mux.Handle("/login",
		common.MakeAPI("login", func(req *http.Request) util.JSONResponse {
			return readers.Login(req, accountDB, deviceDB, cfg)
		}),
	)

	r0mux.Handle("/pushrules/",
		common.MakeAPI("push_rules", func(req *http.Request) util.JSONResponse {
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
		common.MakeAPI("make_filter", func(req *http.Request) util.JSONResponse {
			// TODO: Persist filter and return filter ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	)

	r0mux.Handle("/user/{userID}/filter/{filterID}",
		common.MakeAPI("filter", func(req *http.Request) util.JSONResponse {
			// TODO: Retrieve filter based on ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	)

	// Riot user settings

	r0mux.Handle("/profile/{userID}",
		common.MakeAPI("profile", func(req *http.Request) util.JSONResponse {
			// TODO: Get profile data for user ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	)

	r0mux.Handle("/account/3pid",
		common.MakeAPI("account_3pid", func(req *http.Request) util.JSONResponse {
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
		common.MakeAPI("presence", func(req *http.Request) util.JSONResponse {
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
