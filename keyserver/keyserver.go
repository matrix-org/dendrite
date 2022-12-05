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

package keyserver

import (
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	fedsenderapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/consumers"
	"github.com/matrix-org/dendrite/keyserver/internal"
	"github.com/matrix-org/dendrite/keyserver/inthttp"
	"github.com/matrix-org/dendrite/keyserver/producers"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.KeyInternalAPI) {
	inthttp.AddRoutes(router, intAPI)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite, cfg *config.KeyServer, fedClient fedsenderapi.KeyserverFederationAPI,
) api.KeyInternalAPI {
	js, _ := base.NATS.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)

	db, err := storage.NewDatabase(base, &cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to key server database")
	}
	keyChangeProducer := &producers.KeyChange{
		Topic:     string(cfg.Matrix.JetStream.Prefixed(jetstream.OutputKeyChangeEvent)),
		JetStream: js,
		DB:        db,
	}
	ap := &internal.KeyInternalAPI{
		DB:        db,
		Cfg:       cfg,
		FedClient: fedClient,
		Producer:  keyChangeProducer,
	}
	updater := internal.NewDeviceListUpdater(base.ProcessContext, db, ap, keyChangeProducer, fedClient, 8, cfg.Matrix.ServerName) // 8 workers TODO: configurable
	ap.Updater = updater
	go func() {
		if err := updater.Start(); err != nil {
			logrus.WithError(err).Panicf("failed to start device list updater")
		}
	}()

	dlConsumer := consumers.NewDeviceListUpdateConsumer(
		base.ProcessContext, cfg, js, updater,
	)
	if err := dlConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start device list consumer")
	}

	sigConsumer := consumers.NewSigningKeyUpdateConsumer(
		base.ProcessContext, cfg, js, ap,
	)
	if err := sigConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start signing key consumer")
	}

	return ap
}
