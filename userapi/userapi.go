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
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.UserInternalAPI) {
	inthttp.AddRoutes(router, intAPI)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite, db storage.Database, cfg *config.UserAPI,
	appServices []config.ApplicationService, keyAPI keyapi.KeyInternalAPI,
	rsAPI rsapi.RoomserverInternalAPI, pgClient pushgateway.Client,
) api.UserInternalAPI {
	js, _ := jetstream.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)

	syncProducer := producers.NewSyncAPI(
		db, js,
		// TODO: user API should handle syncs for account data. Right now,
		// it's handled by clientapi, and hence uses its topic. When user
		// API handles it for all account data, we can remove it from
		// here.
		cfg.Matrix.JetStream.TopicFor(jetstream.OutputClientData),
		cfg.Matrix.JetStream.TopicFor(jetstream.OutputNotificationData),
	)

	userAPI := &internal.UserInternalAPI{
		DB:                   db,
		SyncProducer:         syncProducer,
		ServerName:           cfg.Matrix.ServerName,
		AppServices:          appServices,
		KeyAPI:               keyAPI,
		DisableTLSValidation: cfg.PushGatewayDisableTLSValidation,
	}

	readConsumer := consumers.NewOutputReadUpdateConsumer(
		base.ProcessContext, cfg, js, db, pgClient, userAPI, syncProducer,
	)
	if err := readConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start user API read update consumer")
	}

	eventConsumer := consumers.NewOutputStreamEventConsumer(
		base.ProcessContext, cfg, js, db, pgClient, userAPI, rsAPI, syncProducer,
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

	return userAPI
}
