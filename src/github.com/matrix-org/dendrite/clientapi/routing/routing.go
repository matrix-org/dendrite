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
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	opentracing "github.com/opentracing/opentracing-go"
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
	transactionsCache *transactions.Cache,
	tracer opentracing.Tracer,
) {
	apiMux.Handle("/_matrix/client/versions",
		common.MakeExternalAPI(tracer, "versions", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct {
					Versions []string `json:"versions"`
				}{[]string{
					"r0.0.1",
					"r0.1.0",
					"r0.2.0",
					"r0.3.0",
				}},
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()
	v1mux := apiMux.PathPrefix(pathPrefixV1).Subrouter()
	unstableMux := apiMux.PathPrefix(pathPrefixUnstable).Subrouter()

	authData := auth.Data{accountDB, deviceDB, cfg.Derived.ApplicationServices}

	r0mux.Handle("/createRoom",
		common.MakeAuthAPI(tracer, "createRoom", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return CreateRoom(req, device, cfg, producer, accountDB, aliasAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/join/{roomIDOrAlias}",
		common.MakeAuthAPI(tracer, "join", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return JoinRoomByIDOrAlias(
				req, device, vars["roomIDOrAlias"], cfg, federation, producer, queryAPI, aliasAPI, keyRing, accountDB,
			)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/{membership:(?:join|kick|ban|unban|leave|invite)}",
		common.MakeAuthAPI(tracer, "membership", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return SendMembership(req, accountDB, device, vars["roomID"], vars["membership"], cfg, queryAPI, producer)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/send/{eventType}",
		common.MakeAuthAPI(tracer, "send_message", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return SendEvent(req, device, vars["roomID"], vars["eventType"], nil, nil, cfg, queryAPI, producer, nil)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/send/{eventType}/{txnID}",
		common.MakeAuthAPI(tracer, "send_message", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			txnID := vars["txnID"]
			return SendEvent(req, device, vars["roomID"], vars["eventType"], &txnID,
				nil, cfg, queryAPI, producer, transactionsCache)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/state/{eventType:[^/]+/?}",
		common.MakeAuthAPI(tracer, "send_message", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			emptyString := ""
			eventType := vars["eventType"]
			// If there's a trailing slash, remove it
			if strings.HasSuffix(eventType, "/") {
				eventType = eventType[:len(eventType)-1]
			}
			return SendEvent(req, device, vars["roomID"], eventType, nil, &emptyString, cfg, queryAPI, producer, nil)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/state/{eventType}/{stateKey}",
		common.MakeAuthAPI(tracer, "send_message", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			stateKey := vars["stateKey"]
			return SendEvent(req, device, vars["roomID"], vars["eventType"], nil, &stateKey, cfg, queryAPI, producer, nil)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/register", common.MakeExternalAPI(tracer, "register", func(req *http.Request) util.JSONResponse {
		return Register(req, accountDB, deviceDB, &cfg)
	})).Methods(http.MethodPost, http.MethodOptions)

	v1mux.Handle("/register", common.MakeExternalAPI(tracer, "register", func(req *http.Request) util.JSONResponse {
		return LegacyRegister(req, accountDB, deviceDB, &cfg)
	})).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/register/available", common.MakeExternalAPI(tracer, "registerAvailable", func(req *http.Request) util.JSONResponse {
		return RegisterAvailable(req, accountDB)
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/directory/room/{roomAlias}",
		common.MakeAuthAPI(tracer, "directory_room", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return DirectoryRoom(req, vars["roomAlias"], federation, &cfg, aliasAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/directory/room/{roomAlias}",
		common.MakeAuthAPI(tracer, "directory_room", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return SetLocalAlias(req, device, vars["roomAlias"], &cfg, aliasAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/directory/room/{roomAlias}",
		common.MakeAuthAPI(tracer, "directory_room", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return RemoveLocalAlias(req, device, vars["roomAlias"], aliasAPI)
		}),
	).Methods(http.MethodDelete, http.MethodOptions)

	r0mux.Handle("/logout",
		common.MakeAuthAPI(tracer, "logout", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return Logout(req, deviceDB, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/logout/all",
		common.MakeAuthAPI(tracer, "logout", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return LogoutAll(req, deviceDB, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	// Stub endpoints required by Riot

	r0mux.Handle("/login",
		common.MakeExternalAPI(tracer, "login", func(req *http.Request) util.JSONResponse {
			return Login(req, accountDB, deviceDB, cfg)
		}),
	).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	r0mux.Handle("/pushrules/",
		common.MakeExternalAPI(tracer, "push_rules", func(req *http.Request) util.JSONResponse {
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
				Code: http.StatusOK,
				JSON: &res,
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/user/{userId}/filter",
		common.MakeAuthAPI(tracer, "put_filter", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return PutFilter(req, device, accountDB, vars["userId"])
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/user/{userId}/filter/{filterId}",
		common.MakeAuthAPI(tracer, "get_filter", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return GetFilter(req, device, accountDB, vars["userId"], vars["filterId"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	// Riot user settings

	r0mux.Handle("/profile/{userID}",
		common.MakeExternalAPI(tracer, "profile", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return GetProfile(req, accountDB, vars["userID"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/profile/{userID}/avatar_url",
		common.MakeExternalAPI(tracer, "profile_avatar_url", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return GetAvatarURL(req, accountDB, vars["userID"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/profile/{userID}/avatar_url",
		common.MakeAuthAPI(tracer, "profile_avatar_url", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return SetAvatarURL(req, accountDB, device, vars["userID"], userUpdateProducer, &cfg, producer, queryAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	r0mux.Handle("/profile/{userID}/displayname",
		common.MakeExternalAPI(tracer, "profile_displayname", func(req *http.Request) util.JSONResponse {
			vars := mux.Vars(req)
			return GetDisplayName(req, accountDB, vars["userID"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/profile/{userID}/displayname",
		common.MakeAuthAPI(tracer, "profile_displayname", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return SetDisplayName(req, accountDB, device, vars["userID"], userUpdateProducer, &cfg, producer, queryAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	r0mux.Handle("/account/3pid",
		common.MakeAuthAPI(tracer, "account_3pid", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return GetAssociated3PIDs(req, accountDB, device)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/account/3pid",
		common.MakeAuthAPI(tracer, "account_3pid", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return CheckAndSave3PIDAssociation(req, accountDB, device, cfg)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstableMux.Handle("/account/3pid/delete",
		common.MakeAuthAPI(tracer, "account_3pid", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return Forget3PID(req, accountDB)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/{path:(?:account/3pid|register)}/email/requestToken",
		common.MakeExternalAPI(tracer, "account_3pid_request_token", func(req *http.Request) util.JSONResponse {
			return RequestEmailToken(req, accountDB, cfg)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	// Riot logs get flooded unless this is handled
	r0mux.Handle("/presence/{userID}/status",
		common.MakeExternalAPI(tracer, "presence", func(req *http.Request) util.JSONResponse {
			// TODO: Set presence (probably the responsibility of a presence server not clientapi)
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct{}{},
			}
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/voip/turnServer",
		common.MakeAuthAPI(tracer, "turn_server", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return RequestTurnServer(req, device, cfg)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	unstableMux.Handle("/thirdparty/protocols",
		common.MakeExternalAPI(tracer, "thirdparty_protocols", func(req *http.Request) util.JSONResponse {
			// TODO: Return the third party protcols
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct{}{},
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/initialSync",
		common.MakeExternalAPI(tracer, "rooms_initial_sync", func(req *http.Request) util.JSONResponse {
			// TODO: Allow people to peek into rooms.
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.GuestAccessForbidden("Guest access not implemented"),
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/user/{userID}/account_data/{type}",
		common.MakeAuthAPI(tracer, "user_account_data", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return SaveAccountData(req, accountDB, device, vars["userID"], "", vars["type"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/user/{userID}/rooms/{roomID}/account_data/{type}",
		common.MakeAuthAPI(tracer, "user_account_data", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return SaveAccountData(req, accountDB, device, vars["userID"], vars["roomID"], vars["type"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/members",
		common.MakeAuthAPI(tracer, "rooms_members", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return GetMemberships(req, device, vars["roomID"], false, cfg, queryAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/joined_members",
		common.MakeAuthAPI(tracer, "rooms_members", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return GetMemberships(req, device, vars["roomID"], true, cfg, queryAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/read_markers",
		common.MakeExternalAPI(tracer, "rooms_read_markers", func(req *http.Request) util.JSONResponse {
			// TODO: return the read_markers.
			return util.JSONResponse{Code: http.StatusOK, JSON: struct{}{}}
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/typing/{userID}",
		common.MakeExternalAPI(tracer, "rooms_typing", func(req *http.Request) util.JSONResponse {
			// TODO: handling typing
			return util.JSONResponse{Code: http.StatusOK, JSON: struct{}{}}
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/devices",
		common.MakeAuthAPI(tracer, "get_devices", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return GetDevicesByLocalpart(req, deviceDB, device)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/devices/{deviceID}",
		common.MakeAuthAPI(tracer, "get_device", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return GetDeviceByID(req, deviceDB, device, vars["deviceID"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/devices/{deviceID}",
		common.MakeAuthAPI(tracer, "device_data", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			vars := mux.Vars(req)
			return UpdateDeviceByID(req, deviceDB, device, vars["deviceID"])
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	// Stub implementations for sytest
	r0mux.Handle("/events",
		common.MakeExternalAPI(tracer, "events", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{Code: http.StatusOK, JSON: map[string]interface{}{
				"chunk": []interface{}{},
				"start": "",
				"end":   "",
			}}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/initialSync",
		common.MakeExternalAPI(tracer, "initial_sync", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{Code: http.StatusOK, JSON: map[string]interface{}{
				"end": "",
			}}
		}),
	).Methods(http.MethodGet, http.MethodOptions)
}
