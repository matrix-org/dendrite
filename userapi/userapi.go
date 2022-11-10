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

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/internal/pushgateway"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/consumers"
	"github.com/matrix-org/dendrite/userapi/internal"
	"github.com/matrix-org/dendrite/userapi/inthttp"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/util"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.UserInternalAPI) {
	inthttp.AddRoutes(router, intAPI)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite, cfg *config.UserAPI,
	appServices []config.ApplicationService, keyAPI keyapi.UserKeyAPI,
	rsAPI rsapi.UserRoomserverAPI, pgClient pushgateway.Client,
) api.UserInternalAPI {
	js, _ := base.NATS.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)

	db, err := storage.NewUserAPIDatabase(
		base,
		&cfg.AccountDatabase,
		cfg.Matrix.ServerName,
		cfg.BCryptCost,
		cfg.OpenIDTokenLifetimeMS,
		api.DefaultLoginTokenLifetime,
		cfg.Matrix.ServerNotices.LocalPart,
	)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to accounts db")
	}

	syncProducer := producers.NewSyncAPI(
		db, js,
		// TODO: user API should handle syncs for account data. Right now,
		// it's handled by clientapi, and hence uses its topic. When user
		// API handles it for all account data, we can remove it from
		// here.
		cfg.Matrix.JetStream.Prefixed(jetstream.OutputClientData),
		cfg.Matrix.JetStream.Prefixed(jetstream.OutputNotificationData),
	)

	userAPI := &internal.UserInternalAPI{
		DB:                   db,
		SyncProducer:         syncProducer,
		Config:               cfg,
		AppServices:          appServices,
		KeyAPI:               keyAPI,
		RSAPI:                rsAPI,
		DisableTLSValidation: cfg.PushGatewayDisableTLSValidation,
		PgClient:             pgClient,
		Cfg:                  cfg,
	}

	receiptConsumer := consumers.NewOutputReceiptEventConsumer(
		base.ProcessContext, cfg, js, db, syncProducer, pgClient,
	)
	if err := receiptConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start user API receipt consumer")
	}

	eventConsumer := consumers.NewOutputRoomEventConsumer(
		base.ProcessContext, cfg, js, db, pgClient, rsAPI, syncProducer,
	)
	if err := eventConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start user API streamed event consumer")
	}

	var cleanOldNotifs func()
	cleanOldNotifs = func() {
		logrus.Infof("Cleaning old notifications")
		if err := db.DeleteOldNotifications(base.Context()); err != nil {
			logrus.WithError(err).Error("Failed to clean old notifications")
		}
		time.AfterFunc(time.Hour, cleanOldNotifs)
	}
	time.AfterFunc(time.Minute, cleanOldNotifs)

	if base.Cfg.Global.ReportStats.Enabled {
		go util.StartPhoneHomeCollector(time.Now(), base.Cfg, db)
	}

	return userAPI
}
