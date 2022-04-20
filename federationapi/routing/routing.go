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
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

// Setup registers HTTP handlers with the given ServeMux.
// The provided publicAPIMux MUST have `UseEncodedPath()` enabled or else routes will incorrectly
// path unescape twice (once from the router, once from MakeFedAPI). We need to have this enabled
// so we can decode paths like foo/bar%2Fbaz as [foo, bar/baz] - by default it will decode to [foo, bar, baz]
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	fedMux, keyMux, wkMux *mux.Router,
	cfg *config.FederationAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	fsAPI federationAPI.FederationInternalAPI,
	keys gomatrixserverlib.JSONVerifier,
	federation *gomatrixserverlib.FederationClient,
	userAPI userapi.UserInternalAPI,
	keyAPI keyserverAPI.KeyInternalAPI,
	mscCfg *config.MSCs,
	servers federationAPI.ServersInRoomProvider,
	producer *producers.SyncAPIProducer,
) {
	prometheus.MustRegister(
		pduCountTotal, eduCountTotal,
	)

	v2keysmux := keyMux.PathPrefix("/v2").Subrouter()
	v1fedmux := fedMux.PathPrefix("/v1").Subrouter()
	v2fedmux := fedMux.PathPrefix("/v2").Subrouter()

	wakeup := &httputil.FederationWakeups{
		FsAPI: fsAPI,
	}

	localKeys := httputil.MakeExternalAPI("localkeys", func(req *http.Request) util.JSONResponse {
		return LocalKeys(cfg)
	})

	notaryKeys := httputil.MakeExternalAPI("notarykeys", func(req *http.Request) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		var pkReq *gomatrixserverlib.PublicKeyNotaryLookupRequest
		serverName := gomatrixserverlib.ServerName(vars["serverName"])
		keyID := gomatrixserverlib.KeyID(vars["keyID"])
		if serverName != "" && keyID != "" {
			pkReq = &gomatrixserverlib.PublicKeyNotaryLookupRequest{
				ServerKeys: map[gomatrixserverlib.ServerName]map[gomatrixserverlib.KeyID]gomatrixserverlib.PublicKeyNotaryQueryCriteria{
					serverName: {
						keyID: gomatrixserverlib.PublicKeyNotaryQueryCriteria{},
					},
				},
			}
		}
		return NotaryKeys(req, cfg, fsAPI, pkReq)
	})

	if cfg.Matrix.WellKnownServerName != "" {
		logrus.Infof("Setting m.server as %s at /.well-known/matrix/server", cfg.Matrix.WellKnownServerName)
		wkMux.Handle("/server", httputil.MakeExternalAPI("wellknown", func(req *http.Request) util.JSONResponse {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct {
					ServerName string `json:"m.server"`
				}{
					ServerName: cfg.Matrix.WellKnownServerName,
				},
			}
		}),
		).Methods(http.MethodGet, http.MethodOptions)
	}

	// Ignore the {keyID} argument as we only have a single server key so we always
	// return that key.
	// Even if we had more than one server key, we would probably still ignore the
	// {keyID} argument and always return a response containing all of the keys.
	v2keysmux.Handle("/server/{keyID}", localKeys).Methods(http.MethodGet)
	v2keysmux.Handle("/server/", localKeys).Methods(http.MethodGet)
	v2keysmux.Handle("/server", localKeys).Methods(http.MethodGet)
	v2keysmux.Handle("/query", notaryKeys).Methods(http.MethodPost)
	v2keysmux.Handle("/query/{serverName}/{keyID}", notaryKeys).Methods(http.MethodGet)

	mu := internal.NewMutexByRoom()
	v1fedmux.Handle("/send/{txnID}", httputil.MakeFedAPI(
		"federation_send", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return Send(
				httpReq, request, gomatrixserverlib.TransactionID(vars["txnID"]),
				cfg, rsAPI, keyAPI, keys, federation, mu, servers, producer,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/invite/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			return InviteV1(
				httpReq, request, vars["roomID"], vars["eventID"],
				cfg, rsAPI, keys,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v2fedmux.Handle("/invite/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			return InviteV2(
				httpReq, request, vars["roomID"], vars["eventID"],
				cfg, rsAPI, keys,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/3pid/onbind", httputil.MakeExternalAPI("3pid_onbind",
		func(req *http.Request) util.JSONResponse {
			return CreateInvitesFrom3PIDInvites(req, rsAPI, cfg, federation, userAPI)
		},
	)).Methods(http.MethodPost, http.MethodOptions)

	v1fedmux.Handle("/exchange_third_party_invite/{roomID}", httputil.MakeFedAPI(
		"exchange_third_party_invite", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return ExchangeThirdPartyInvite(
				httpReq, request, vars["roomID"], rsAPI, cfg, federation,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/event/{eventID}", httputil.MakeFedAPI(
		"federation_get_event", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return GetEvent(
				httpReq.Context(), request, rsAPI, vars["eventID"], cfg.Matrix.ServerName,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/state/{roomID}", httputil.MakeFedAPI(
		"federation_get_state", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetState(
				httpReq.Context(), request, rsAPI, vars["roomID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/state_ids/{roomID}", httputil.MakeFedAPI(
		"federation_get_state_ids", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetStateIDs(
				httpReq.Context(), request, rsAPI, vars["roomID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/event_auth/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_get_event_auth", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetEventAuth(
				httpReq.Context(), request, rsAPI, vars["roomID"], vars["eventID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/query/directory", httputil.MakeFedAPI(
		"federation_query_room_alias", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return RoomAliasToID(
				httpReq, federation, cfg, rsAPI, fsAPI,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/query/profile", httputil.MakeFedAPI(
		"federation_query_profile", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return GetProfile(
				httpReq, userAPI, cfg,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/user/devices/{userID}", httputil.MakeFedAPI(
		"federation_user_devices", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return GetUserDevices(
				httpReq, keyAPI, vars["userID"],
			)
		},
	)).Methods(http.MethodGet)

	if mscCfg.Enabled("msc2444") {
		v1fedmux.Handle("/peek/{roomID}/{peekID}", httputil.MakeFedAPI(
			"federation_peek", cfg.Matrix.ServerName, keys, wakeup,
			func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
				if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
					return util.JSONResponse{
						Code: http.StatusForbidden,
						JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
					}
				}
				roomID := vars["roomID"]
				peekID := vars["peekID"]
				queryVars := httpReq.URL.Query()
				remoteVersions := []gomatrixserverlib.RoomVersion{}
				if vers, ok := queryVars["ver"]; ok {
					// The remote side supplied a ?ver= so use that to build up the list
					// of supported room versions
					for _, v := range vers {
						remoteVersions = append(remoteVersions, gomatrixserverlib.RoomVersion(v))
					}
				} else {
					// The remote side didn't supply a ?ver= so just assume that they only
					// support room version 1
					remoteVersions = append(remoteVersions, gomatrixserverlib.RoomVersionV1)
				}
				return Peek(
					httpReq, request, cfg, rsAPI, roomID, peekID, remoteVersions,
				)
			},
		)).Methods(http.MethodPut, http.MethodDelete)
	}

	v1fedmux.Handle("/make_join/{roomID}/{userID}", httputil.MakeFedAPI(
		"federation_make_join", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			roomID := vars["roomID"]
			userID := vars["userID"]
			queryVars := httpReq.URL.Query()
			remoteVersions := []gomatrixserverlib.RoomVersion{}
			if vers, ok := queryVars["ver"]; ok {
				// The remote side supplied a ?ver= so use that to build up the list
				// of supported room versions
				for _, v := range vers {
					remoteVersions = append(remoteVersions, gomatrixserverlib.RoomVersion(v))
				}
			} else {
				// The remote side didn't supply a ?ver= so just assume that they only
				// support room version 1, as per the spec
				// https://matrix.org/docs/spec/server_server/r0.1.3#get-matrix-federation-v1-make-join-roomid-userid
				remoteVersions = append(remoteVersions, gomatrixserverlib.RoomVersionV1)
			}
			return MakeJoin(
				httpReq, request, cfg, rsAPI, roomID, userID, remoteVersions,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/send_join/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_send_join", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			roomID := vars["roomID"]
			eventID := vars["eventID"]
			res := SendJoin(
				httpReq, request, cfg, rsAPI, keys, roomID, eventID,
			)
			// not all responses get wrapped in [code, body]
			var body interface{}
			body = []interface{}{
				res.Code, res.JSON,
			}
			jerr, ok := res.JSON.(*jsonerror.MatrixError)
			if ok {
				body = jerr
			}

			return util.JSONResponse{
				Headers: res.Headers,
				Code:    res.Code,
				JSON:    body,
			}
		},
	)).Methods(http.MethodPut)

	v2fedmux.Handle("/send_join/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_send_join", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			roomID := vars["roomID"]
			eventID := vars["eventID"]
			return SendJoin(
				httpReq, request, cfg, rsAPI, keys, roomID, eventID,
			)
		},
	)).Methods(http.MethodPut)

	v1fedmux.Handle("/make_leave/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_make_leave", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			roomID := vars["roomID"]
			eventID := vars["eventID"]
			return MakeLeave(
				httpReq, request, cfg, rsAPI, roomID, eventID,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/send_leave/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_send_leave", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			roomID := vars["roomID"]
			eventID := vars["eventID"]
			res := SendLeave(
				httpReq, request, cfg, rsAPI, keys, roomID, eventID,
			)
			// not all responses get wrapped in [code, body]
			var body interface{}
			body = []interface{}{
				res.Code, res.JSON,
			}
			jerr, ok := res.JSON.(*jsonerror.MatrixError)
			if ok {
				body = jerr
			}

			return util.JSONResponse{
				Headers: res.Headers,
				Code:    res.Code,
				JSON:    body,
			}
		},
	)).Methods(http.MethodPut)

	v2fedmux.Handle("/send_leave/{roomID}/{eventID}", httputil.MakeFedAPI(
		"federation_send_leave", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			roomID := vars["roomID"]
			eventID := vars["eventID"]
			return SendLeave(
				httpReq, request, cfg, rsAPI, keys, roomID, eventID,
			)
		},
	)).Methods(http.MethodPut)

	v1fedmux.Handle("/version", httputil.MakeExternalAPI(
		"federation_version",
		func(httpReq *http.Request) util.JSONResponse {
			return Version()
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/get_missing_events/{roomID}", httputil.MakeFedAPI(
		"federation_get_missing_events", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetMissingEvents(httpReq, request, rsAPI, vars["roomID"])
		},
	)).Methods(http.MethodPost)

	v1fedmux.Handle("/backfill/{roomID}", httputil.MakeFedAPI(
		"federation_backfill", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: jsonerror.Forbidden("Forbidden by server ACLs"),
				}
			}
			return Backfill(httpReq, request, rsAPI, vars["roomID"], cfg)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/publicRooms",
		httputil.MakeExternalAPI("federation_public_rooms", func(req *http.Request) util.JSONResponse {
			return GetPostPublicRooms(req, rsAPI)
		}),
	).Methods(http.MethodGet, http.MethodPost)

	v1fedmux.Handle("/user/keys/claim", httputil.MakeFedAPI(
		"federation_keys_claim", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return ClaimOneTimeKeys(httpReq, request, keyAPI, cfg.Matrix.ServerName)
		},
	)).Methods(http.MethodPost)

	v1fedmux.Handle("/user/keys/query", httputil.MakeFedAPI(
		"federation_keys_query", cfg.Matrix.ServerName, keys, wakeup,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest, vars map[string]string) util.JSONResponse {
			return QueryDeviceKeys(httpReq, request, keyAPI, cfg.Matrix.ServerName)
		},
	)).Methods(http.MethodPost)

	v1fedmux.Handle("/openid/userinfo",
		httputil.MakeExternalAPI("federation_openid_userinfo", func(req *http.Request) util.JSONResponse {
			return GetOpenIDUserInfo(req, userAPI)
		}),
	).Methods(http.MethodGet)
}
