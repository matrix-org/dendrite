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
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/consumers"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/internal/transactions"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// AddPublicRoutes sets up and registers HTTP handlers for the ClientAPI component.
func AddPublicRoutes(
	router *mux.Router,
	base *basecomponent.BaseDendrite,
	deviceDB devices.Database,
	accountsDB accounts.Database,
	federation *gomatrixserverlib.FederationClient,
	keyRing *gomatrixserverlib.KeyRing,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	eduInputAPI eduServerAPI.EDUServerInputAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	transactionsCache *transactions.Cache,
	fsAPI federationSenderAPI.FederationSenderInternalAPI,
) {
	roomserverProducer := producers.NewRoomserverProducer(rsAPI)
	eduProducer := producers.NewEDUServerProducer(eduInputAPI)

	userUpdateProducer := &producers.UserUpdateProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.UserUpdates),
	}

	syncProducer := &producers.SyncAPIProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.OutputClientData),
	}

	consumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, accountsDB, rsAPI,
	)
	if err := consumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	routing.Setup(
		router, base.Cfg, roomserverProducer, rsAPI, asAPI,
		accountsDB, deviceDB, federation, *keyRing, userUpdateProducer,
		syncProducer, eduProducer, transactionsCache, fsAPI,
	)
}
