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
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/transactions"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/dendrite/userapi/storage/devices"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const pathPrefixV1 = "/client/api/v1"
const pathPrefixR0 = "/client/r0"
const pathPrefixUnstable = "/client/unstable"

// Setup registers HTTP handlers with the given ServeMux. It also supplies the given http.Client
// to clients which need to make outbound HTTP requests.
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	publicAPIMux *mux.Router, cfg *config.Dendrite,
	eduAPI eduServerAPI.EDUServerInputAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	accountDB accounts.Database,
	deviceDB devices.Database,
	userAPI api.UserInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	syncProducer *producers.SyncAPIProducer,
	transactionsCache *transactions.Cache,
	federationSender federationSenderAPI.FederationSenderInternalAPI,
) {

	publicAPIMux.Handle("/client/versions",
		httputil.MakeExternalAPI("versions", func(req *http.Request) util.JSONResponse {
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

	r0mux := publicAPIMux.PathPrefix(pathPrefixR0).Subrouter()
	v1mux := publicAPIMux.PathPrefix(pathPrefixV1).Subrouter()
	unstableMux := publicAPIMux.PathPrefix(pathPrefixUnstable).Subrouter()

	r0mux.Handle("/createRoom",
		httputil.MakeAuthAPI("createRoom", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return CreateRoom(req, device, cfg, accountDB, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/join/{roomIDOrAlias}",
		httputil.MakeAuthAPI(gomatrixserverlib.Join, userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return JoinRoomByIDOrAlias(
				req, device, rsAPI, accountDB, vars["roomIDOrAlias"],
			)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/joined_rooms",
		httputil.MakeAuthAPI("joined_rooms", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return GetJoinedRooms(req, device, accountDB)
		}),
	).Methods(http.MethodGet, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/join",
		httputil.MakeAuthAPI(gomatrixserverlib.Join, userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return JoinRoomByIDOrAlias(
				req, device, rsAPI, accountDB, vars["roomID"],
			)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/leave",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return LeaveRoomByID(
				req, device, rsAPI, vars["roomID"],
			)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/ban",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendBan(req, accountDB, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/invite",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendInvite(req, accountDB, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/kick",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendKick(req, accountDB, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/unban",
		httputil.MakeAuthAPI("membership", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendUnban(req, accountDB, device, vars["roomID"], cfg, rsAPI, asAPI)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/send/{eventType}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendEvent(req, device, vars["roomID"], vars["eventType"], nil, nil, cfg, rsAPI, nil)
		}),
	).Methods(http.MethodPost, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/send/{eventType}/{txnID}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			txnID := vars["txnID"]
			return SendEvent(req, device, vars["roomID"], vars["eventType"], &txnID,
				nil, cfg, rsAPI, transactionsCache)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	r0mux.Handle("/rooms/{roomID}/event/{eventID}",
		httputil.MakeAuthAPI("rooms_get_event", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetEvent(req, device, vars["roomID"], vars["eventID"], cfg, rsAPI, federation)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state", httputil.MakeAuthAPI("room_state", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		return OnIncomingStateRequest(req.Context(), rsAPI, vars["roomID"])
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state/{type:[^/]+/?}", httputil.MakeAuthAPI("room_state", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		// If there's a trailing slash, remove it
		eventType := vars["type"]
		if strings.HasSuffix(eventType, "/") {
			eventType = eventType[:len(eventType)-1]
		}
		eventFormat := req.URL.Query().Get("format") == "event"
		return OnIncomingStateTypeRequest(req.Context(), rsAPI, vars["roomID"], eventType, "", eventFormat)
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state/{type}/{stateKey}", httputil.MakeAuthAPI("room_state", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		eventFormat := req.URL.Query().Get("format") == "event"
		return OnIncomingStateTypeRequest(req.Context(), rsAPI, vars["roomID"], vars["type"], vars["stateKey"], eventFormat)
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state/{eventType:[^/]+/?}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			emptyString := ""
			eventType := vars["eventType"]
			// If there's a trailing slash, remove it
			if strings.HasSuffix(eventType, "/") {
				eventType = eventType[:len(eventType)-1]
			}
			return SendEvent(req, device, vars["roomID"], eventType, nil, &emptyString, cfg, rsAPI, nil)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/state/{eventType}/{stateKey}",
		httputil.MakeAuthAPI("send_message", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			stateKey := vars["stateKey"]
			return SendEvent(req, device, vars["roomID"], vars["eventType"], nil, &stateKey, cfg, rsAPI, nil)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/register", httputil.MakeExternalAPI("register", func(req *http.Request) util.JSONResponse {
		return Register(req, userAPI, accountDB, cfg)
	})).Methods(http.MethodPost, http.MethodOptions)

	v1mux.Handle("/register", httputil.MakeExternalAPI("register", func(req *http.Request) util.JSONResponse {
		return LegacyRegister(req, userAPI, cfg)
	})).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/register/available", httputil.MakeExternalAPI("registerAvailable", func(req *http.Request) util.JSONResponse {
		return RegisterAvailable(req, cfg, accountDB)
	})).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/directory/room/{roomAlias}",
		httputil.MakeExternalAPI("directory_room", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return DirectoryRoom(req, vars["roomAlias"], federation, cfg, rsAPI, federationSender)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/directory/room/{roomAlias}",
		httputil.MakeAuthAPI("directory_room", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetLocalAlias(req, device, vars["roomAlias"], cfg, rsAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/directory/room/{roomAlias}",
		httputil.MakeAuthAPI("directory_room", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return RemoveLocalAlias(req, device, vars["roomAlias"], rsAPI)
		}),
	).Methods(http.MethodDelete, http.MethodOptions)

	r0mux.Handle("/logout",
		httputil.MakeAuthAPI("logout", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return Logout(req, deviceDB, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/logout/all",
		httputil.MakeAuthAPI("logout", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return LogoutAll(req, deviceDB, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/typing/{userID}",
		httputil.MakeAuthAPI("rooms_typing", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SendTyping(req, device, vars["roomID"], vars["userID"], accountDB, eduAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/sendToDevice/{eventType}/{txnID}",
		httputil.MakeAuthAPI("send_to_device", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			txnID := vars["txnID"]
			return SendToDevice(req, device, eduAPI, transactionsCache, vars["eventType"], &txnID)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	// This is only here because sytest refers to /unstable for this endpoint
	// rather than r0. It's an exact duplicate of the above handler.
	// TODO: Remove this if/when sytest is fixed!
	unstableMux.Handle("/sendToDevice/{eventType}/{txnID}",
		httputil.MakeAuthAPI("send_to_device", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			txnID := vars["txnID"]
			return SendToDevice(req, device, eduAPI, transactionsCache, vars["eventType"], &txnID)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/account/whoami",
		httputil.MakeAuthAPI("whoami", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return Whoami(req, device)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	// Stub endpoints required by Riot

	r0mux.Handle("/login",
		httputil.MakeExternalAPI("login", func(req *http.Request) util.JSONResponse {
			return Login(req, accountDB, deviceDB, cfg)
		}),
	).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	r0mux.Handle("/auth/{authType}/fallback/web",
		httputil.MakeHTMLAPI("auth_fallback", func(w http.ResponseWriter, req *http.Request) *util.JSONResponse {
			vars := mux.Vars(req)
			return AuthFallback(w, req, vars["authType"], cfg)
		}),
	).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	r0mux.Handle("/pushrules/",
		httputil.MakeExternalAPI("push_rules", func(req *http.Request) util.JSONResponse {
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
		httputil.MakeAuthAPI("put_filter", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return PutFilter(req, device, accountDB, vars["userId"])
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/user/{userId}/filter/{filterId}",
		httputil.MakeAuthAPI("get_filter", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetFilter(req, device, accountDB, vars["userId"], vars["filterId"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	// Riot user settings

	r0mux.Handle("/profile/{userID}",
		httputil.MakeExternalAPI("profile", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetProfile(req, accountDB, cfg, vars["userID"], asAPI, federation)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/profile/{userID}/avatar_url",
		httputil.MakeExternalAPI("profile_avatar_url", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetAvatarURL(req, accountDB, cfg, vars["userID"], asAPI, federation)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/profile/{userID}/avatar_url",
		httputil.MakeAuthAPI("profile_avatar_url", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetAvatarURL(req, accountDB, device, vars["userID"], cfg, rsAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	r0mux.Handle("/profile/{userID}/displayname",
		httputil.MakeExternalAPI("profile_displayname", func(req *http.Request) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetDisplayName(req, accountDB, cfg, vars["userID"], asAPI, federation)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/profile/{userID}/displayname",
		httputil.MakeAuthAPI("profile_displayname", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SetDisplayName(req, accountDB, device, vars["userID"], cfg, rsAPI)
		}),
	).Methods(http.MethodPut, http.MethodOptions)
	// Browsers use the OPTIONS HTTP method to check if the CORS policy allows
	// PUT requests, so we need to allow this method

	r0mux.Handle("/account/3pid",
		httputil.MakeAuthAPI("account_3pid", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return GetAssociated3PIDs(req, accountDB, device)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/account/3pid",
		httputil.MakeAuthAPI("account_3pid", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return CheckAndSave3PIDAssociation(req, accountDB, device, cfg)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	unstableMux.Handle("/account/3pid/delete",
		httputil.MakeAuthAPI("account_3pid", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return Forget3PID(req, accountDB)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/{path:(?:account/3pid|register)}/email/requestToken",
		httputil.MakeExternalAPI("account_3pid_request_token", func(req *http.Request) util.JSONResponse {
			return RequestEmailToken(req, accountDB, cfg)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	// Riot logs get flooded unless this is handled
	r0mux.Handle("/presence/{userID}/status",
		httputil.MakeExternalAPI("presence", func(req *http.Request) util.JSONResponse {
			// TODO: Set presence (probably the responsibility of a presence server not clientapi)
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct{}{},
			}
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/voip/turnServer",
		httputil.MakeAuthAPI("turn_server", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return RequestTurnServer(req, device, cfg)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/thirdparty/protocols",
		httputil.MakeExternalAPI("thirdparty_protocols", func(req *http.Request) util.JSONResponse {
			// TODO: Return the third party protcols
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct{}{},
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/initialSync",
		httputil.MakeExternalAPI("rooms_initial_sync", func(req *http.Request) util.JSONResponse {
			// TODO: Allow people to peek into rooms.
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.GuestAccessForbidden("Guest access not implemented"),
			}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/user/{userID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SaveAccountData(req, userAPI, device, vars["userID"], "", vars["type"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/user/{userID}/rooms/{roomID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return SaveAccountData(req, userAPI, device, vars["userID"], vars["roomID"], vars["type"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/user/{userID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetAccountData(req, userAPI, device, vars["userID"], "", vars["type"])
		}),
	).Methods(http.MethodGet)

	r0mux.Handle("/user/{userID}/rooms/{roomID}/account_data/{type}",
		httputil.MakeAuthAPI("user_account_data", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetAccountData(req, userAPI, device, vars["userID"], vars["roomID"], vars["type"])
		}),
	).Methods(http.MethodGet)

	r0mux.Handle("/rooms/{roomID}/members",
		httputil.MakeAuthAPI("rooms_members", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetMemberships(req, device, vars["roomID"], false, cfg, rsAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/joined_members",
		httputil.MakeAuthAPI("rooms_members", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetMemberships(req, device, vars["roomID"], true, cfg, rsAPI)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/rooms/{roomID}/read_markers",
		httputil.MakeExternalAPI("rooms_read_markers", func(req *http.Request) util.JSONResponse {
			// TODO: return the read_markers.
			return util.JSONResponse{Code: http.StatusOK, JSON: struct{}{}}
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	r0mux.Handle("/devices",
		httputil.MakeAuthAPI("get_devices", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return GetDevicesByLocalpart(req, deviceDB, device)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/devices/{deviceID}",
		httputil.MakeAuthAPI("get_device", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetDeviceByID(req, deviceDB, device, vars["deviceID"])
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/devices/{deviceID}",
		httputil.MakeAuthAPI("device_data", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return UpdateDeviceByID(req, deviceDB, device, vars["deviceID"])
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/devices/{deviceID}",
		httputil.MakeAuthAPI("delete_device", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return DeleteDeviceById(req, deviceDB, device, vars["deviceID"])
		}),
	).Methods(http.MethodDelete, http.MethodOptions)

	r0mux.Handle("/delete_devices",
		httputil.MakeAuthAPI("delete_devices", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return DeleteDevices(req, deviceDB, device)
		}),
	).Methods(http.MethodPost, http.MethodOptions)

	// Stub implementations for sytest
	r0mux.Handle("/events",
		httputil.MakeExternalAPI("events", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{Code: http.StatusOK, JSON: map[string]interface{}{
				"chunk": []interface{}{},
				"start": "",
				"end":   "",
			}}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/initialSync",
		httputil.MakeExternalAPI("initial_sync", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{Code: http.StatusOK, JSON: map[string]interface{}{
				"end": "",
			}}
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/user/{userId}/rooms/{roomId}/tags",
		httputil.MakeAuthAPI("get_tags", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetTags(req, userAPI, device, vars["userId"], vars["roomId"], syncProducer)
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/user/{userId}/rooms/{roomId}/tags/{tag}",
		httputil.MakeAuthAPI("put_tag", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return PutTag(req, userAPI, device, vars["userId"], vars["roomId"], vars["tag"], syncProducer)
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/user/{userId}/rooms/{roomId}/tags/{tag}",
		httputil.MakeAuthAPI("delete_tag", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return DeleteTag(req, userAPI, device, vars["userId"], vars["roomId"], vars["tag"], syncProducer)
		}),
	).Methods(http.MethodDelete, http.MethodOptions)

	r0mux.Handle("/capabilities",
		httputil.MakeAuthAPI("capabilities", userAPI, func(req *http.Request, device *api.Device) util.JSONResponse {
			return GetCapabilities(req, rsAPI)
		}),
	).Methods(http.MethodGet)
}
