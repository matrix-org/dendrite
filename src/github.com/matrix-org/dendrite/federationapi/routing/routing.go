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
	"time"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const (
	pathPrefixV2Keys       = "/_matrix/key/v2"
	pathPrefixV1Federation = "/_matrix/federation/v1"
)

// Setup registers HTTP handlers with the given ServeMux.
func Setup(
	apiMux *mux.Router,
	cfg config.Dendrite,
	query api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
	federation *gomatrixserverlib.FederationClient,
	accountDB *accounts.Database,
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
	v2keysmux.Handle("/server/{keyID}", localKeys).Methods("GET")
	v2keysmux.Handle("/server/", localKeys).Methods("GET")

	v1fedmux.Handle("/send/{txnID}/", common.MakeFedAPI(
		"federation_send", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars := mux.Vars(httpReq)
			return Send(
				httpReq, request, gomatrixserverlib.TransactionID(vars["txnID"]),
				cfg, query, producer, keys, federation,
			)
		},
	)).Methods("PUT", "OPTIONS")

	v1fedmux.Handle("/invite/{roomID}/{eventID}", common.MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars := mux.Vars(httpReq)
			return Invite(
				httpReq, request, vars["roomID"], vars["eventID"],
				cfg, producer, keys,
			)
		},
	)).Methods("PUT", "OPTIONS")

	v1fedmux.Handle("/3pid/onbind", common.MakeExternalAPI("3pid_onbind",
		func(req *http.Request) util.JSONResponse {
			return CreateInvitesFrom3PIDInvites(req, query, cfg, producer, federation, accountDB)
		},
	)).Methods("POST", "OPTIONS")

	v1fedmux.Handle("/exchange_third_party_invite/{roomID}", common.MakeFedAPI(
		"exchange_third_party_invite", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars := mux.Vars(httpReq)
			return ExchangeThirdPartyInvite(
				httpReq, request, vars["roomID"], query, cfg, federation, producer,
			)
		},
	)).Methods("PUT", "OPTIONS")

	v1fedmux.Handle("/event/{eventID}", common.MakeFedAPI(
		"federation_get_event", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars := mux.Vars(httpReq)
			return GetEvent(
				httpReq.Context(), request, cfg, query, time.Now(), keys, vars["eventID"],
			)
		},
	)).Methods("GET")

	v1fedmux.Handle("/query/profile", common.MakeFedAPI(
		"federation_query_profile", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			return GetProfile(
				httpReq, accountDB, cfg,
			)
		},
	)).Methods("GET")

	v1fedmux.Handle("/version", common.MakeExternalAPI(
		"federation_version",
		func(httpReq *http.Request) util.JSONResponse {
			return Version()
		},
	)).Methods("GET")
}
