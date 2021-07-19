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
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/transactions"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/kafka"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"
)

// AddPublicRoutes sets up and registers HTTP handlers for the ClientAPI component.
func AddPublicRoutes(
	router *mux.Router,
	synapseAdminRouter *mux.Router,
	cfg *config.ClientAPI,
	accountsDB accounts.Database,
	federation *gomatrixserverlib.FederationClient,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	eduInputAPI eduServerAPI.EDUServerInputAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	transactionsCache *transactions.Cache,
	fsAPI federationSenderAPI.FederationSenderInternalAPI,
	userAPI userapi.UserInternalAPI,
	keyAPI keyserverAPI.KeyInternalAPI,
	extRoomsProvider api.ExtraPublicRoomsProvider,
	mscCfg *config.MSCs,
) {
	_, producer := kafka.SetupConsumerProducer(&cfg.Matrix.Kafka)

	syncProducer := &producers.SyncAPIProducer{
		Producer: producer,
		Topic:    cfg.Matrix.Kafka.TopicFor(config.TopicOutputClientData),
	}

	routing.Setup(
		router, synapseAdminRouter, cfg, eduInputAPI, rsAPI, asAPI,
		accountsDB, userAPI, federation,
		syncProducer, transactionsCache, fsAPI, keyAPI, extRoomsProvider, mscCfg,
	)
}
