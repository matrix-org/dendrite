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

package main

import (
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/consumers"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/clientapi/routing"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/keydb"
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
) {
	roomserverProducer := producers.NewRoomserverProducer(base.InputAPI())

	userUpdateProducer := &producers.UserUpdateProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.UserUpdates),
	}

	syncProducer := &producers.SyncAPIProducer{
		Producer: base.KafkaProducer,
		Topic:    string(base.Cfg.Kafka.Topics.OutputClientData),
	}

	consumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, accountsDB, base.QueryAPI(),
	)
	if err := consumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	routing.Setup(
		base.APIMux, *base.Cfg, roomserverProducer,
		base.QueryAPI(), base.AliasAPI(), accountsDB, deviceDB,
		federation, *keyRing,
		userUpdateProducer, syncProducer,
	)
}

func main() {
	base := basecomponent.NewBaseDendrite("ClientAPI")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	keyDB := base.CreateKeyDB()
	federation := base.CreateFederationClient()
	keyRing := keydb.CreateKeyRing(federation.Client, keyDB)

	SetupClientAPIComponent(
		base, deviceDB, accountDB, federation, &keyRing,
	)

	base.SetupAndServeHTTP(string(base.Cfg.Listen.ClientAPI))
}
