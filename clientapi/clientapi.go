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
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/consumers"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/transactions"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	typingServerAPI "github.com/matrix-org/dendrite/typingserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// SetupClientAPIComponent sets up and registers HTTP handlers for the ClientAPI
// component.
func SetupClientAPIComponent(
	base *basecomponent.BaseDendrite,
	deviceDB *devices.Database,
	accountsDB *accounts.Database,
	federation *gomatrixserverlib.FederationClient,
	keyRing *gomatrixserverlib.KeyRing,
	aliasAPI roomserverAPI.RoomserverAliasAPI,
	inputAPI roomserverAPI.RoomserverInputAPI,
	queryAPI roomserverAPI.RoomserverQueryAPI,
	typingInputAPI typingServerAPI.TypingServerInputAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	transactionsCache *transactions.Cache,
	fedSenderAPI federationSenderAPI.FederationSenderQueryAPI,
) {
	roomserverProducer := producers.NewRoomserverProducer(inputAPI)
	typingProducer := producers.NewTypingServerProducer(typingInputAPI)

	userUpdateProducer := &producers.UserUpdateProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.UserUpdates),
	}

	syncProducer := &producers.SyncAPIProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.OutputClientData),
	}

	consumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, accountsDB, queryAPI,
	)
	if err := consumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	routing.Setup(
		base.APIMux, base.Cfg, roomserverProducer, queryAPI, aliasAPI, asAPI,
		accountsDB, deviceDB, federation, *keyRing, userUpdateProducer,
		syncProducer, typingProducer, transactionsCache, fedSenderAPI,
	)
}
