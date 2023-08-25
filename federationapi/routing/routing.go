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
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	fedInternal "github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/producers"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
)

const (
	SendRouteName           = "Send"
	QueryDirectoryRouteName = "QueryDirectory"
	QueryProfileRouteName   = "QueryProfile"
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
	routers httputil.Routers,
	dendriteCfg *config.Dendrite,
	rsAPI roomserverAPI.FederationRoomserverAPI,
	fsAPI *fedInternal.FederationInternalAPI,
	keys gomatrixserverlib.JSONVerifier,
	federation fclient.FederationClient,
	userAPI userapi.FederationUserAPI,
	mscCfg *config.MSCs,
	producer *producers.SyncAPIProducer, enableMetrics bool,
) {
	fedMux := routers.Federation
	keyMux := routers.Keys
	wkMux := routers.WellKnown
	cfg := &dendriteCfg.FederationAPI

	if enableMetrics {
		prometheus.MustRegister(
			internal.PDUCountTotal, internal.EDUCountTotal,
		)
	}

	v2keysmux := keyMux.PathPrefix("/v2").Subrouter()
	v1fedmux := fedMux.PathPrefix("/v1").Subrouter()
	v2fedmux := fedMux.PathPrefix("/v2").Subrouter()
	v3fedmux := fedMux.PathPrefix("/v3").Subrouter()

	wakeup := &FederationWakeups{
		FsAPI: fsAPI,
	}

	localKeys := httputil.MakeExternalAPI("localkeys", func(req *http.Request) util.JSONResponse {
		return LocalKeys(cfg, spec.ServerName(req.Host))
	})

	notaryKeys := httputil.MakeExternalAPI("notarykeys", func(req *http.Request) util.JSONResponse {
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.ErrorResponse(err)
		}
		var pkReq *gomatrixserverlib.PublicKeyNotaryLookupRequest
		serverName := spec.ServerName(vars["serverName"])
		keyID := gomatrixserverlib.KeyID(vars["keyID"])
		if serverName != "" && keyID != "" {
			pkReq = &gomatrixserverlib.PublicKeyNotaryLookupRequest{
				ServerKeys: map[spec.ServerName]map[gomatrixserverlib.KeyID]gomatrixserverlib.PublicKeyNotaryQueryCriteria{
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
	v1fedmux.Handle("/send/{txnID}", MakeFedAPI(
		"federation_send", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return Send(
				httpReq, request, gomatrixserverlib.TransactionID(vars["txnID"]),
				cfg, rsAPI, userAPI, keys, federation, mu, producer,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions).Name(SendRouteName)

	v1fedmux.Handle("/invite/{roomID}/{eventID}", MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}

			roomID, err := spec.NewRoomID(vars["roomID"])
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid RoomID"),
				}
			}
			return InviteV1(
				httpReq, request, *roomID, vars["eventID"],
				cfg, rsAPI, keys,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v2fedmux.Handle("/invite/{roomID}/{eventID}", MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}

			roomID, err := spec.NewRoomID(vars["roomID"])
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid RoomID"),
				}
			}
			return InviteV2(
				httpReq, request, *roomID, vars["eventID"],
				cfg, rsAPI, keys,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v3fedmux.Handle("/invite/{roomID}/{userID}", MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}

			userID, err := spec.NewUserID(vars["userID"], true)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid UserID"),
				}
			}
			roomID, err := spec.NewRoomID(vars["roomID"])
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid RoomID"),
				}
			}
			return InviteV3(
				httpReq, request, *roomID, *userID,
				cfg, rsAPI, keys,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/3pid/onbind", httputil.MakeExternalAPI("3pid_onbind",
		func(req *http.Request) util.JSONResponse {
			return CreateInvitesFrom3PIDInvites(req, rsAPI, cfg, federation, userAPI)
		},
	)).Methods(http.MethodPost, http.MethodOptions)

	v1fedmux.Handle("/exchange_third_party_invite/{roomID}", MakeFedAPI(
		"exchange_third_party_invite", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return ExchangeThirdPartyInvite(
				httpReq, request, vars["roomID"], rsAPI, cfg, federation,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/event/{eventID}", MakeFedAPI(
		"federation_get_event", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return GetEvent(
				httpReq.Context(), request, rsAPI, vars["eventID"], cfg.Matrix.ServerName,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/state/{roomID}", MakeFedAPI(
		"federation_get_state", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetState(
				httpReq.Context(), request, rsAPI, vars["roomID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/state_ids/{roomID}", MakeFedAPI(
		"federation_get_state_ids", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetStateIDs(
				httpReq.Context(), request, rsAPI, vars["roomID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/event_auth/{roomID}/{eventID}", MakeFedAPI(
		"federation_get_event_auth", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetEventAuth(
				httpReq.Context(), request, rsAPI, vars["roomID"], vars["eventID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/query/directory", MakeFedAPI(
		"federation_query_room_alias", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return RoomAliasToID(
				httpReq, federation, cfg, rsAPI, fsAPI,
			)
		},
	)).Methods(http.MethodGet).Name(QueryDirectoryRouteName)

	v1fedmux.Handle("/query/profile", MakeFedAPI(
		"federation_query_profile", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return GetProfile(
				httpReq, userAPI, cfg,
			)
		},
	)).Methods(http.MethodGet).Name(QueryProfileRouteName)

	v1fedmux.Handle("/user/devices/{userID}", MakeFedAPI(
		"federation_user_devices", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return GetUserDevices(
				httpReq, userAPI, vars["userID"],
			)
		},
	)).Methods(http.MethodGet)

	if mscCfg.Enabled("msc2444") {
		v1fedmux.Handle("/peek/{roomID}/{peekID}", MakeFedAPI(
			"federation_peek", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
			func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
				if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
					return util.JSONResponse{
						Code: http.StatusForbidden,
						JSON: spec.Forbidden("Forbidden by server ACLs"),
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

	v1fedmux.Handle("/make_join/{roomID}/{userID}", MakeFedAPI(
		"federation_make_join", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
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

			userID, err := spec.NewUserID(vars["userID"], true)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid UserID"),
				}
			}
			roomID, err := spec.NewRoomID(vars["roomID"])
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid RoomID"),
				}
			}

			logrus.Debugf("Processing make_join for user %s, room %s", userID.String(), roomID.String())
			return MakeJoin(
				httpReq, request, cfg, rsAPI, *roomID, *userID, remoteVersions,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/send_join/{roomID}/{eventID}", MakeFedAPI(
		"federation_send_join", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
			eventID := vars["eventID"]
			roomID, err := spec.NewRoomID(vars["roomID"])
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid RoomID"),
				}
			}

			res := SendJoin(
				httpReq, request, cfg, rsAPI, keys, *roomID, eventID,
			)
			// not all responses get wrapped in [code, body]
			var body interface{}
			body = []interface{}{
				res.Code, res.JSON,
			}
			jerr, ok := res.JSON.(spec.MatrixError)
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

	v2fedmux.Handle("/send_join/{roomID}/{eventID}", MakeFedAPI(
		"federation_send_join", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
			eventID := vars["eventID"]
			roomID, err := spec.NewRoomID(vars["roomID"])
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid RoomID"),
				}
			}

			return SendJoin(
				httpReq, request, cfg, rsAPI, keys, *roomID, eventID,
			)
		},
	)).Methods(http.MethodPut)

	v1fedmux.Handle("/make_leave/{roomID}/{userID}", MakeFedAPI(
		"federation_make_leave", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
			roomID, err := spec.NewRoomID(vars["roomID"])
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid RoomID"),
				}
			}
			userID, err := spec.NewUserID(vars["userID"], true)
			if err != nil {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.InvalidParam("Invalid UserID"),
				}
			}
			return MakeLeave(
				httpReq, request, cfg, rsAPI, *roomID, *userID,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/send_leave/{roomID}/{eventID}", MakeFedAPI(
		"federation_send_leave", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
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
			jerr, ok := res.JSON.(spec.MatrixError)
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

	v2fedmux.Handle("/send_leave/{roomID}/{eventID}", MakeFedAPI(
		"federation_send_leave", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
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

	v1fedmux.Handle("/get_missing_events/{roomID}", MakeFedAPI(
		"federation_get_missing_events", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
				}
			}
			return GetMissingEvents(httpReq, request, rsAPI, vars["roomID"])
		},
	)).Methods(http.MethodPost)

	v1fedmux.Handle("/backfill/{roomID}", MakeFedAPI(
		"federation_backfill", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			if roomserverAPI.IsServerBannedFromRoom(httpReq.Context(), rsAPI, vars["roomID"], request.Origin()) {
				return util.JSONResponse{
					Code: http.StatusForbidden,
					JSON: spec.Forbidden("Forbidden by server ACLs"),
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

	v1fedmux.Handle("/user/keys/claim", MakeFedAPI(
		"federation_keys_claim", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return ClaimOneTimeKeys(httpReq, request, userAPI, cfg.Matrix.ServerName)
		},
	)).Methods(http.MethodPost)

	v1fedmux.Handle("/user/keys/query", MakeFedAPI(
		"federation_keys_query", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return QueryDeviceKeys(httpReq, request, userAPI, cfg.Matrix.ServerName)
		},
	)).Methods(http.MethodPost)

	v1fedmux.Handle("/openid/userinfo",
		httputil.MakeExternalAPI("federation_openid_userinfo", func(req *http.Request) util.JSONResponse {
			return GetOpenIDUserInfo(req, userAPI)
		}),
	).Methods(http.MethodGet)

	v1fedmux.Handle("/hierarchy/{roomID}", MakeFedAPI(
		"federation_room_hierarchy", cfg.Matrix.ServerName, cfg.Matrix.IsLocalServerName, keys, wakeup,
		func(httpReq *http.Request, request *fclient.FederationRequest, vars map[string]string) util.JSONResponse {
			return QueryRoomHierarchy(httpReq, request, vars["roomID"], rsAPI)
		},
	)).Methods(http.MethodGet)
}

func ErrorIfLocalServerNotInRoom(
	ctx context.Context,
	rsAPI api.FederationRoomserverAPI,
	roomID string,
) *util.JSONResponse {
	// Check if we think we're in this room. If we aren't then
	// we won't waste CPU cycles serving this request.
	joinedReq := &api.QueryServerJoinedToRoomRequest{
		RoomID: roomID,
	}
	joinedRes := &api.QueryServerJoinedToRoomResponse{}
	if err := rsAPI.QueryServerJoinedToRoom(ctx, joinedReq, joinedRes); err != nil {
		res := util.ErrorResponse(err)
		return &res
	}
	if !joinedRes.IsInRoom {
		return &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound(fmt.Sprintf("This server is not joined to room %s", roomID)),
		}
	}
	return nil
}

// MakeFedAPI makes an http.Handler that checks matrix federation authentication.
func MakeFedAPI(
	metricsName string, serverName spec.ServerName,
	isLocalServerName func(spec.ServerName) bool,
	keyRing gomatrixserverlib.JSONVerifier,
	wakeup *FederationWakeups,
	f func(*http.Request, *fclient.FederationRequest, map[string]string) util.JSONResponse,
) http.Handler {
	h := func(req *http.Request) util.JSONResponse {
		fedReq, errResp := fclient.VerifyHTTPRequest(
			req, time.Now(), serverName, isLocalServerName, keyRing,
		)
		if fedReq == nil {
			return errResp
		}
		// add the user to Sentry, if enabled
		hub := sentry.GetHubFromContext(req.Context())
		if hub != nil {
			hub.Scope().SetTag("origin", string(fedReq.Origin()))
			hub.Scope().SetTag("uri", fedReq.RequestURI())
		}
		defer func() {
			if r := recover(); r != nil {
				if hub != nil {
					hub.CaptureException(fmt.Errorf("%s panicked", req.URL.Path))
				}
				// re-panic to return the 500
				panic(r)
			}
		}()
		go wakeup.Wakeup(req.Context(), fedReq.Origin())
		vars, err := httputil.URLDecodeMapValues(mux.Vars(req))
		if err != nil {
			return util.MatrixErrorResponse(400, string(spec.ErrorUnrecognized), "badly encoded query params")
		}

		jsonRes := f(req, fedReq, vars)
		// do not log 4xx as errors as they are client fails, not server fails
		if hub != nil && jsonRes.Code >= 500 {
			hub.Scope().SetExtra("response", jsonRes)
			hub.CaptureException(fmt.Errorf("%s returned HTTP %d", req.URL.Path, jsonRes.Code))
		}
		return jsonRes
	}
	return httputil.MakeExternalAPI(metricsName, h)
}

type FederationWakeups struct {
	FsAPI   *fedInternal.FederationInternalAPI
	origins sync.Map
}

func (f *FederationWakeups) Wakeup(ctx context.Context, origin spec.ServerName) {
	key, keyok := f.origins.Load(origin)
	if keyok {
		lastTime, ok := key.(time.Time)
		if ok && time.Since(lastTime) < time.Minute {
			return
		}
	}
	f.FsAPI.MarkServersAlive([]spec.ServerName{origin})
	f.origins.Store(origin, time.Now())
}
