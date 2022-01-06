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

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/eduserver/cache"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/syncapi/consumers"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/routing"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/streams"
	"github.com/matrix-org/dendrite/syncapi/sync"
)

// AddPublicRoutes sets up and registers HTTP handlers for the SyncAPI
// component.
func AddPublicRoutes(
	process *process.ProcessContext,
	router *mux.Router,
	userAPI userapi.UserInternalAPI,
	rsAPI api.RoomserverInternalAPI,
	keyAPI keyapi.KeyInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	cfg *config.SyncAPI,
) {
	js, consumer, _ := jetstream.Prepare(&cfg.Matrix.JetStream)

	syncDB, err := storage.NewSyncServerDatasource(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to sync db")
	}

	eduCache := cache.New()
	streams := streams.NewSyncStreamProviders(syncDB, userAPI, rsAPI, keyAPI, eduCache)
	notifier := notifier.NewNotifier(streams.Latest(context.Background()))
	if err = notifier.Load(context.Background(), syncDB); err != nil {
		logrus.WithError(err).Panicf("failed to load notifier ")
	}

	requestPool := sync.NewRequestPool(syncDB, cfg, userAPI, keyAPI, rsAPI, streams, notifier)

	keyChangeConsumer := consumers.NewOutputKeyChangeEventConsumer(
		process, cfg.Matrix.ServerName, cfg.Matrix.JetStream.TopicFor(jetstream.OutputKeyChangeEvent),
		consumer, keyAPI, rsAPI, syncDB, notifier,
		streams.DeviceListStreamProvider,
	)
	if err = keyChangeConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start key change consumer")
	}

	roomConsumer := consumers.NewOutputRoomEventConsumer(
		process, cfg, js, syncDB, notifier, streams.PDUStreamProvider,
		streams.InviteStreamProvider, rsAPI,
	)
	if err = roomConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}

	clientConsumer := consumers.NewOutputClientDataConsumer(
		process, cfg, js, syncDB, notifier, streams.AccountDataStreamProvider,
	)
	if err = clientConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start client data consumer")
	}

	typingConsumer := consumers.NewOutputTypingEventConsumer(
		process, cfg, js, syncDB, eduCache, notifier, streams.TypingStreamProvider,
	)
	if err = typingConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start typing consumer")
	}

	sendToDeviceConsumer := consumers.NewOutputSendToDeviceEventConsumer(
		process, cfg, js, syncDB, notifier, streams.SendToDeviceStreamProvider,
	)
	if err = sendToDeviceConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start send-to-device consumer")
	}

	receiptConsumer := consumers.NewOutputReceiptEventConsumer(
		process, cfg, js, syncDB, notifier, streams.ReceiptStreamProvider,
	)
	if err = receiptConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start receipts consumer")
	}

	routing.Setup(router, requestPool, syncDB, userAPI, federation, rsAPI, cfg)
}
