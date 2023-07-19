// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package userapi

import (
	"time"

	fedsenderapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/sirupsen/logrus"

	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/consumers"
	"github.com/matrix-org/dendrite/userapi/internal"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/util"
)

// NewInternalAPI returns a concrete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
//
// Creating a new instance of the user API requires a roomserver API with a federation API set
// using its `SetFederationAPI` method, other you may get nil-dereference errors.
func NewInternalAPI(
	processContext *process.ProcessContext,
	dendriteCfg *config.Dendrite,
	cm *sqlutil.Connections,
	natsInstance *jetstream.NATSInstance,
	rsAPI rsapi.UserRoomserverAPI,
	fedClient fedsenderapi.KeyserverFederationAPI,
) *internal.UserInternalAPI {
	js, _ := natsInstance.Prepare(processContext, &dendriteCfg.Global.JetStream)
	appServices := dendriteCfg.Derived.ApplicationServices

	pgClient := pushgateway.NewHTTPClient(dendriteCfg.UserAPI.PushGatewayDisableTLSValidation)

	db, err := storage.NewUserDatabase(
		processContext.Context(),
		cm,
		&dendriteCfg.UserAPI.AccountDatabase,
		dendriteCfg.Global.ServerName,
		dendriteCfg.UserAPI.BCryptCost,
		dendriteCfg.UserAPI.OpenIDTokenLifetimeMS,
		api.DefaultLoginTokenLifetime,
		dendriteCfg.UserAPI.Matrix.ServerNotices.LocalPart,
	)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	keyDB, err := storage.NewKeyDatabase(cm, &dendriteCfg.KeyServer.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to key db")
	}

	syncProducer := producers.NewSyncAPI(
		db, js,
		// TODO: user API should handle syncs for account data. Right now,
		// it's handled by clientapi, and hence uses its topic. When user
		// API handles it for all account data, we can remove it from
		// here.
		dendriteCfg.Global.JetStream.Prefixed(jetstream.OutputClientData),
		dendriteCfg.Global.JetStream.Prefixed(jetstream.OutputNotificationData),
	)
	keyChangeProducer := &producers.KeyChange{
		Topic:     dendriteCfg.Global.JetStream.Prefixed(jetstream.OutputKeyChangeEvent),
		JetStream: js,
		DB:        keyDB,
	}

	userAPI := &internal.UserInternalAPI{
		DB:                   db,
		KeyDatabase:          keyDB,
		SyncProducer:         syncProducer,
		KeyChangeProducer:    keyChangeProducer,
		Config:               &dendriteCfg.UserAPI,
		AppServices:          appServices,
		RSAPI:                rsAPI,
		DisableTLSValidation: dendriteCfg.UserAPI.PushGatewayDisableTLSValidation,
		PgClient:             pgClient,
		FedClient:            fedClient,
	}

	updater := internal.NewDeviceListUpdater(processContext, keyDB, userAPI, keyChangeProducer, fedClient, 8, rsAPI, dendriteCfg.Global.ServerName) // 8 workers TODO: configurable
	userAPI.Updater = updater
	// Remove users which we don't share a room with anymore
	if err := updater.CleanUp(); err != nil {
		logrus.WithError(err).Error("failed to cleanup stale device lists")
	}

	go func() {
		if err := updater.Start(); err != nil {
			logrus.WithError(err).Panicf("failed to start device list updater")
		}
	}()

	dlConsumer := consumers.NewDeviceListUpdateConsumer(
		processContext, &dendriteCfg.UserAPI, js, updater,
	)
	if err := dlConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start device list consumer")
	}

	sigConsumer := consumers.NewSigningKeyUpdateConsumer(
		processContext, &dendriteCfg.UserAPI, js, userAPI,
	)
	if err := sigConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start signing key consumer")
	}

	receiptConsumer := consumers.NewOutputReceiptEventConsumer(
		processContext, &dendriteCfg.UserAPI, js, db, syncProducer, pgClient,
	)
	if err := receiptConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start user API receipt consumer")
	}

	eventConsumer := consumers.NewOutputRoomEventConsumer(
		processContext, &dendriteCfg.UserAPI, js, db, pgClient, rsAPI, syncProducer,
	)
	if err := eventConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start user API streamed event consumer")
	}

	var cleanOldNotifs func()
	cleanOldNotifs = func() {
		logrus.Infof("Cleaning old notifications")
		if err := db.DeleteOldNotifications(processContext.Context()); err != nil {
			logrus.WithError(err).Error("Failed to clean old notifications")
		}
		time.AfterFunc(time.Hour, cleanOldNotifs)
	}
	time.AfterFunc(time.Minute, cleanOldNotifs)

	if dendriteCfg.Global.ReportStats.Enabled {
		go util.StartPhoneHomeCollector(time.Now(), dendriteCfg, db)
	}

	return userAPI
}
