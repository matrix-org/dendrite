// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package userapi

import (
	"time"

	fedsenderapi "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/federationapi/statistics"
	"github.com/element-hq/dendrite/internal/pushgateway"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"

	rsapi "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/element-hq/dendrite/userapi/consumers"
	"github.com/element-hq/dendrite/userapi/internal"
	"github.com/element-hq/dendrite/userapi/producers"
	"github.com/element-hq/dendrite/userapi/storage"
	"github.com/element-hq/dendrite/userapi/util"
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
	enableMetrics bool,
	blacklistedOrBackingOffFn func(s spec.ServerName) (*statistics.ServerStatistics, error),
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

	updater := internal.NewDeviceListUpdater(processContext, keyDB, userAPI, keyChangeProducer, fedClient, dendriteCfg.UserAPI.WorkerCount, rsAPI, dendriteCfg.Global.ServerName, enableMetrics, blacklistedOrBackingOffFn)
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
