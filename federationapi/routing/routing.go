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
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const (
	pathPrefixV2Keys       = "/_matrix/key/v2"
	pathPrefixV1Federation = "/_matrix/federation/v1"
)

// Setup registers HTTP handlers with the given ServeMux.
//
// Due to Setup being used to call many other functions, a gocyclo nolint is
// applied:
// nolint: gocyclo
func Setup(
	apiMux *mux.Router,
	cfg config.Dendrite,
	query roomserverAPI.RoomserverQueryAPI,
	aliasAPI roomserverAPI.RoomserverAliasAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
	federation *gomatrixserverlib.FederationClient,
	accountDB *accounts.Database,
	deviceDB *devices.Database,
) {
	v2keysmux := apiMux.PathPrefix(pathPrefixV2Keys).Subrouter()
	v1fedmux := apiMux.PathPrefix(pathPrefixV1Federation).Subrouter()

	localKeys := common.MakeExternalAPI("localkeys", func(req *http.Request) util.JSONResponse {
		return LocalKeys(cfg)
	})

	// Ignore the {keyID} argument as we only have a single server key so we always
	// return that key.
	// Even if we had more than one server key, we would probably still ignore the
	// {keyID} argument and always return a response containing all of the keys.
	v2keysmux.Handle("/server/{keyID}", localKeys).Methods(http.MethodGet)
	v2keysmux.Handle("/server/", localKeys).Methods(http.MethodGet)
	v2keysmux.Handle("/server", localKeys).Methods(http.MethodGet)

	v1fedmux.Handle("/send/{txnID}", common.MakeFedAPI(
		"federation_send", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return Send(
				httpReq, request, gomatrixserverlib.TransactionID(vars["txnID"]),
				cfg, query, producer, keys, federation,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/invite/{roomID}/{eventID}", common.MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return Invite(
				httpReq, request, vars["roomID"], vars["eventID"],
				cfg, producer, keys,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/3pid/onbind", common.MakeExternalAPI("3pid_onbind",
		func(req *http.Request) util.JSONResponse {
			return CreateInvitesFrom3PIDInvites(req, query, asAPI, cfg, producer, federation, accountDB)
		},
	)).Methods(http.MethodPost, http.MethodOptions)

	v1fedmux.Handle("/exchange_third_party_invite/{roomID}", common.MakeFedAPI(
		"exchange_third_party_invite", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return ExchangeThirdPartyInvite(
				httpReq, request, vars["roomID"], query, cfg, federation, producer,
			)
		},
	)).Methods(http.MethodPut, http.MethodOptions)

	v1fedmux.Handle("/event/{eventID}", common.MakeFedAPI(
		"federation_get_event", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetEvent(
				httpReq.Context(), request, query, vars["eventID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/state/{roomID}", common.MakeFedAPI(
		"federation_get_event_auth", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetState(
				httpReq.Context(), request, query, vars["roomID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/state_ids/{roomID}", common.MakeFedAPI(
		"federation_get_event_auth", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetStateIDs(
				httpReq.Context(), request, query, vars["roomID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/query/directory", common.MakeFedAPI(
		"federation_query_room_alias", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			return RoomAliasToID(
				httpReq, federation, cfg, aliasAPI,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/query/profile", common.MakeFedAPI(
		"federation_query_profile", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			return GetProfile(
				httpReq, accountDB, cfg, asAPI,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/user/devices/{userID}", common.MakeFedAPI(
		"federation_user_devices", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetUserDevices(
				httpReq, deviceDB, vars["userID"],
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/make_join/{roomID}/{userID}", common.MakeFedAPI(
		"federation_make_join", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			roomID := vars["roomID"]
			userID := vars["userID"]
			return MakeJoin(
				httpReq, request, cfg, query, roomID, userID,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/send_join/{roomID}/{userID}", common.MakeFedAPI(
		"federation_send_join", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			roomID := vars["roomID"]
			userID := vars["userID"]
			return SendJoin(
				httpReq, request, cfg, query, producer, keys, roomID, userID,
			)
		},
	)).Methods(http.MethodPut)

	v1fedmux.Handle("/make_leave/{roomID}/{userID}", common.MakeFedAPI(
		"federation_make_leave", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			roomID := vars["roomID"]
			userID := vars["userID"]
			return MakeLeave(
				httpReq, request, cfg, query, roomID, userID,
			)
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/send_leave/{roomID}/{userID}", common.MakeFedAPI(
		"federation_send_leave", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			roomID := vars["roomID"]
			userID := vars["userID"]
			return SendLeave(
				httpReq, request, cfg, producer, keys, roomID, userID,
			)
		},
	)).Methods(http.MethodPut)

	v1fedmux.Handle("/version", common.MakeExternalAPI(
		"federation_version",
		func(httpReq *http.Request) util.JSONResponse {
			return Version()
		},
	)).Methods(http.MethodGet)

	v1fedmux.Handle("/get_missing_events/{roomID}", common.MakeFedAPI(
		"federation_get_missing_events", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return GetMissingEvents(httpReq, request, query, vars["roomID"])
		},
	)).Methods(http.MethodPost)

	v1fedmux.Handle("/backfill/{roomID}", common.MakeFedAPI(
		"federation_backfill", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars, err := common.URLDecodeMapValues(mux.Vars(httpReq))
			if err != nil {
				return util.ErrorResponse(err)
			}
			return Backfill(httpReq, request, query, vars["roomID"], cfg)
		},
	)).Methods(http.MethodGet)
}
