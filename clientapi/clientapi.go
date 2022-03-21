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

package clientapi

import (
	"github.com/gorilla/mux"
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/api"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/transactions"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	userdb "github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

// AddPublicRoutes sets up and registers HTTP handlers for the ClientAPI component.
func AddPublicRoutes(
	process *process.ProcessContext,
	router *mux.Router,
	synapseAdminRouter *mux.Router,
	cfg *config.ClientAPI,
	accountsDB userdb.Database,
	federation *gomatrixserverlib.FederationClient,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	eduInputAPI eduServerAPI.EDUServerInputAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	transactionsCache *transactions.Cache,
	fsAPI federationAPI.FederationInternalAPI,
	userAPI userapi.UserInternalAPI,
	keyAPI keyserverAPI.KeyInternalAPI,
	extRoomsProvider api.ExtraPublicRoomsProvider,
	mscCfg *config.MSCs,
) {
	js, _ := jetstream.Prepare(process, &cfg.Matrix.JetStream)

	syncProducer := &producers.SyncAPIProducer{
		JetStream: js,
		Topic:     cfg.Matrix.JetStream.TopicFor(jetstream.OutputClientData),
	}

	routing.Setup(
		router, synapseAdminRouter, cfg, eduInputAPI, rsAPI, asAPI,
		accountsDB, userAPI, federation,
		syncProducer, transactionsCache, fsAPI, keyAPI,
		extRoomsProvider, mscCfg,
	)
}
