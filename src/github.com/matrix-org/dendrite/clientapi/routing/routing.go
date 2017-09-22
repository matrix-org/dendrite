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
	"strings"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/readers"
	"github.com/matrix-org/dendrite/clientapi/writers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const pathPrefixV1 = "/_matrix/client/api/v1"
const pathPrefixR0 = "/_matrix/client/r0"
const pathPrefixUnstable = "/_matrix/client/unstable"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
func Setup(
	apiMux *mux.Router, cfg config.Dendrite,
	producer *producers.RoomserverProducer, queryAPI api.RoomserverQueryAPI,
	aliasAPI api.RoomserverAliasAPI,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
	federation *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.KeyRing,
	userUpdateProducer *producers.UserUpdateProducer,
	syncProducer *producers.SyncAPIProducer,
) {

	apiMux.Handle("/_matrix/client/versions",
		common.MakeAPI("versions", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{
				Code: 200,
				JSON: struct {
					Versions []string `json:"versions"`
				}{[]string{
					"r0.0.1",
					"r0.1.0",
					"r0.2.0",
				}},
			}
		}),
	).Methods("GET")

	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	v1mux := apiMux.PathPrefix(pathPrefixV1).Subrouter()
	unstableMux := apiMux.PathPrefix(pathPrefixUnstable).Subrouter()

	r0mux.Handle("/createRoom",
		common.MakeAuthAPI("createRoom", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return writers.CreateRoom(req, device, cfg, producer, accountDB)
		}),
	).Methods("POST", "OPTIONS")
	r0mux.Handle("/join/{roomIDOrAlias}",
		common.MakeAuthAPI("join", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.JoinRoomByIDOrAlias(
				req, device, vars["roomIDOrAlias"], cfg, federation, producer, queryAPI, aliasAPI, keyRing, accountDB,
			)
		}),
	).Methods("POST", "OPTIONS")
	r0mux.Handle("/rooms/{roomID}/{membership:(?:join|kick|ban|unban|leave|invite)}",
		common.MakeAuthAPI("membership", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.SendMembership(req, accountDB, device, vars["roomID"], vars["membership"], cfg, queryAPI, producer)
		}),
	).Methods("POST", "OPTIONS")
	r0mux.Handle("/rooms/{roomID}/send/{eventType}/{txnID}",
		common.MakeAuthAPI("send_message", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return writers.SendEvent(req, device, vars["roomID"], vars["eventType"], vars["txnID"], nil, cfg, queryAPI, producer)
		}),
	).Methods("PUT", "OPTIONS")
	r0mux.Handle("/rooms/{roomID}/state/{eventType:[^/]+/?}",
		common.MakeAuthAPI("send_message", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			emptyString := ""
			eventType := vars["eventType"]
			// If there's a trailing slash, remove it
			if strings.HasSuffix(eventType, "/") {
				eventType = eventType[:len(eventType)-1]
			}
			return writers.SendEvent(req, device, vars["roomID"], eventType, "", &emptyString, cfg, queryAPI, producer)
		}),
	).Methods("PUT", "OPTIONS")
	r0mux.Handle("/rooms/{roomID}/state/{eventType}/{stateKey}",
		common.MakeAuthAPI("send_message", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			stateKey := vars["stateKey"]
			return writers.SendEvent(req, device, vars["roomID"], vars["eventType"], "", &stateKey, cfg, queryAPI, producer)
		}),
	).Methods("PUT", "OPTIONS")

	r0mux.Handle("/register", common.MakeAPI("register", func(req *http.Request) util.JSONResponse {
		return writers.Register(req, accountDB, deviceDB, &cfg)
	})).Methods("POST", "OPTIONS")

	v1mux.Handle("/register", common.MakeAPI("register", func(req *http.Request) util.JSONResponse {
		return writers.LegacyRegister(req, accountDB, deviceDB, &cfg)
	})).Methods("POST", "OPTIONS")

	r0mux.Handle("/directory/room/{roomAlias}",
		common.MakeAuthAPI("directory_room", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.DirectoryRoom(req, vars["roomAlias"], federation, &cfg, aliasAPI)
		}),
	).Methods("GET")

	r0mux.Handle("/directory/room/{roomAlias}",
		common.MakeAuthAPI("directory_room", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.SetLocalAlias(req, device, vars["roomAlias"], &cfg, aliasAPI)
		}),
	).Methods("PUT", "OPTIONS")

	r0mux.Handle("/directory/room/{roomAlias}",
		common.MakeAuthAPI("directory_room", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.RemoveLocalAlias(req, device, vars["roomAlias"], aliasAPI)
		}),
	).Methods("DELETE")

	r0mux.Handle("/logout",
		common.MakeAuthAPI("logout", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return readers.Logout(req, deviceDB, device)
		}),
	).Methods("POST", "OPTIONS")

	// Stub endpoints required by Riot

	r0mux.Handle("/login",
		common.MakeAPI("login", func(req *http.Request) util.JSONResponse {
			return readers.Login(req, accountDB, deviceDB, cfg)
		}),
	).Methods("POST", "OPTIONS")

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
	).Methods("GET")

	r0mux.Handle("/user/{userID}/filter",
		common.MakeAPI("make_filter", func(req *http.Request) util.JSONResponse {
			// TODO: Persist filter and return filter ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	).Methods("POST", "OPTIONS")

	r0mux.Handle("/user/{userID}/filter/{filterID}",
		common.MakeAPI("filter", func(req *http.Request) util.JSONResponse {
			// TODO: Retrieve filter based on ID
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	).Methods("GET")

	// Riot user settings

	r0mux.Handle("/profile/{userID}",
		common.MakeAPI("profile", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.GetProfile(req, accountDB, vars["userID"])
		}),
	).Methods("GET")

	r0mux.Handle("/profile/{userID}/avatar_url",
		common.MakeAPI("profile_avatar_url", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.GetAvatarURL(req, accountDB, vars["userID"])
		}),
	).Methods("GET")

	r0mux.Handle("/profile/{userID}/avatar_url",
		common.MakeAuthAPI("profile_avatar_url", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.SetAvatarURL(req, accountDB, device, vars["userID"], userUpdateProducer, &cfg, producer, queryAPI)
		}),
	).Methods("PUT", "OPTIONS")
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	r0mux.Handle("/profile/{userID}/displayname",
		common.MakeAPI("profile_displayname", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.GetDisplayName(req, accountDB, vars["userID"])
		}),
	).Methods("GET")

	r0mux.Handle("/profile/{userID}/displayname",
		common.MakeAuthAPI("profile_displayname", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.SetDisplayName(req, accountDB, device, vars["userID"], userUpdateProducer, &cfg, producer, queryAPI)
		}),
	).Methods("PUT", "OPTIONS")
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	r0mux.Handle("/account/3pid",
		common.MakeAuthAPI("account_3pid", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return readers.GetAssociated3PIDs(req, accountDB, device)
		}),
	).Methods("GET")

	r0mux.Handle("/account/3pid",
		common.MakeAuthAPI("account_3pid", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return readers.CheckAndSave3PIDAssociation(req, accountDB, device, cfg)
		}),
	).Methods("POST", "OPTIONS")

	unstableMux.Handle("/account/3pid/delete",
		common.MakeAuthAPI("account_3pid", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return readers.Forget3PID(req, accountDB)
		}),
	).Methods("POST", "OPTIONS")

	r0mux.Handle("/{path:(?:account/3pid|register)}/email/requestToken",
		common.MakeAPI("account_3pid_request_token", func(req *http.Request) util.JSONResponse {
			return readers.RequestEmailToken(req, accountDB, cfg)
		}),
	).Methods("POST", "OPTIONS")

	// Riot logs get flooded unless this is handled
	r0mux.Handle("/presence/{userID}/status",
		common.MakeAPI("presence", func(req *http.Request) util.JSONResponse {
			// TODO: Set presence (probably the responsibility of a presence server not clientapi)
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	).Methods("PUT", "OPTIONS")

	r0mux.Handle("/voip/turnServer",
		common.MakeAPI("turn_server", func(req *http.Request) util.JSONResponse {
			// TODO: Return credentials for a turn server if one is configured.
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	).Methods("GET")

	unstableMux.Handle("/thirdparty/protocols",
		common.MakeAPI("thirdparty_protocols", func(req *http.Request) util.JSONResponse {
			// TODO: Return the third party protcols
			return util.JSONResponse{
				Code: 200,
				JSON: struct{}{},
			}
		}),
	).Methods("GET")

	r0mux.Handle("/rooms/{roomID}/initialSync",
		common.MakeAPI("rooms_initial_sync", func(req *http.Request) util.JSONResponse {
			// TODO: Allow people to peek into rooms.
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.GuestAccessForbidden("Guest access not implemented"),
			}
		}),
	).Methods("GET")

	r0mux.Handle("/user/{userID}/account_data/{type}",
		common.MakeAuthAPI("user_account_data", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.SaveAccountData(req, accountDB, device, vars["userID"], "", vars["type"], syncProducer)
		}),
	).Methods("PUT", "OPTIONS")

	r0mux.Handle("/user/{userID}/rooms/{roomID}/account_data/{type}",
		common.MakeAuthAPI("user_account_data", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.SaveAccountData(req, accountDB, device, vars["userID"], vars["roomID"], vars["type"], syncProducer)
		}),
	).Methods("PUT", "OPTIONS")

	r0mux.Handle("/rooms/{roomID}/members",
		common.MakeAuthAPI("rooms_members", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.GetMemberships(req, device, vars["roomID"], false, cfg, queryAPI)
		}),
	).Methods("GET")

	r0mux.Handle("/rooms/{roomID}/joined_members",
		common.MakeAuthAPI("rooms_members", deviceDB, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return readers.GetMemberships(req, device, vars["roomID"], true, cfg, queryAPI)
		}),
	).Methods("GET")

	r0mux.Handle("/rooms/{roomID}/read_markers",
		common.MakeAPI("rooms_read_markers", func(req *http.Request) util.JSONResponse {
			// TODO: return the read_markers.
			return util.JSONResponse{Code: 200, JSON: struct{}{}}
		}),
	).Methods("POST", "OPTIONS")

	r0mux.Handle("/rooms/{roomID}/typing/{userID}",
		common.MakeAPI("rooms_typing", func(req *http.Request) util.JSONResponse {
			// TODO: handling typing
			return util.JSONResponse{Code: 200, JSON: struct{}{}}
		}),
	).Methods("PUT", "OPTIONS")
}
