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
	"github.com/matrix-org/dendrite/clientapi/consumers"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/internal/transactions"
	"github.com/sirupsen/logrus"
)

// SetupClientAPIComponent sets up and registers HTTP handlers for the ClientAPI
// component.
func SetupClientAPIComponent(
	base *setup.Base,
	transactionsCache *transactions.Cache,
) {
	roomserverProducer := producers.NewRoomserverProducer(base.RoomserverAPI())
	eduProducer := producers.NewEDUServerProducer(base.EDUServer())

	userUpdateProducer := &producers.UserUpdateProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.UserUpdates),
	}

	syncProducer := &producers.SyncAPIProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.OutputClientData),
	}

	consumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, base.AccountDB, base.RoomserverAPI(),
	)
	if err := consumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	keyRing := base.ServerKeyAPI().KeyRing()
	routing.Setup(
		base.PublicAPIMux, base.Cfg, roomserverProducer, base.RoomserverAPI(), base.AppserviceAPI(),
		base.AccountDB, base.DeviceDB, base.FederationClient, *keyRing, userUpdateProducer,
		syncProducer, eduProducer, transactionsCache, base.FederationSender(),
	)
}
