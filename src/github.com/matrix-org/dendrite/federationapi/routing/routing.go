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
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/federationapi/readers"
	"github.com/matrix-org/dendrite/federationapi/writers"
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
) {
	v2keysmux := apiMux.PathPrefix(pathPrefixV2Keys).Subrouter()
	v1fedmux := apiMux.PathPrefix(pathPrefixV1Federation).Subrouter()

	localKeys := common.MakeAPI("localkeys", func(req *http.Request) util.JSONResponse {
		return readers.LocalKeys(req, cfg)
	})

	// Ignore the {keyID} argument as we only have a single server key so we always
	// return that key.
	// Even if we had more than one server key, we would probably still ignore the
	// {keyID} argument and always return a response containing all of the keys.
	v2keysmux.Handle("/server/{keyID}", localKeys)
	v2keysmux.Handle("/server/", localKeys)

	v1fedmux.Handle("/send/{txnID}/", common.MakeFedAPI(
		"federation_send", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars := mux.Vars(httpReq)
			return writers.Send(
				httpReq, request, gomatrixserverlib.TransactionID(vars["txnID"]),
				cfg, query, producer, keys, federation,
			)
		},
	))

	v1fedmux.Handle("/invite/{roomID}/{eventID}", common.MakeFedAPI(
		"federation_invite", cfg.Matrix.ServerName, keys,
		func(httpReq *http.Request, request *gomatrixserverlib.FederationRequest) util.JSONResponse {
			vars := mux.Vars(httpReq)
			return writers.Invite(
				httpReq, request, vars["roomID"], vars["eventID"],
				cfg, producer, keys,
			)
		},
	))
}
