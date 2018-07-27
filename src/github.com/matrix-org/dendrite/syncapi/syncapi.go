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

package syncapi

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	encryptoapi "github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/syncapi/consumers"
	"github.com/matrix-org/dendrite/syncapi/routing"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
)

// SetupSyncAPIComponent sets up and registers HTTP handlers for the SyncAPI
// component.
func SetupSyncAPIComponent(
	base *basecomponent.BaseDendrite,
	deviceDB *devices.Database,
	accountsDB *accounts.Database,
	queryAPI api.RoomserverQueryAPI,
	encryptDB *encryptoapi.Database,
) {
	syncDB, err := storage.NewSyncServerDatabase(string(base.Cfg.Database.SyncAPI))
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to sync db")
	}

	pos, err := syncDB.SyncStreamPosition(context.Background())
	if err != nil {
		logrus.WithError(err).Panicf("failed to get stream position")
	}

	notifier := sync.NewNotifier(types.StreamPosition(pos))
	err = notifier.Load(context.Background(), syncDB)
	if err != nil {
		logrus.WithError(err).Panicf("failed to start notifier")
	}

	requestPool := sync.NewRequestPool(syncDB, notifier, accountsDB)

	roomConsumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, notifier, syncDB, queryAPI,
	)
	if err = roomConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	clientConsumer := consumers.NewOutputClientDataConsumer(
		base.Cfg, base.KafkaConsumer, notifier, syncDB,
	)
	if err = clientConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start client data consumer")
	}

	routing.Setup(base.APIMux, requestPool, syncDB, deviceDB, notifier, encryptDB)
}
