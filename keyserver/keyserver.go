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
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.KeyInternalAPI) {
	inthttp.AddRoutes(router, intAPI)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *base.BaseDendrite, cfg *config.KeyServer, fedClient fedsenderapi.FederationClient,
) api.KeyInternalAPI {
	_, consumer, producer := jetstream.Prepare(&cfg.Matrix.JetStream)

	db, err := storage.NewDatabase(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to key server database")
	}
	keyChangeProducer := &producers.KeyChange{
		Topic:    string(cfg.Matrix.JetStream.TopicFor(jetstream.OutputKeyChangeEvent)),
		Producer: producer,
		DB:       db,
	}
	ap := &internal.KeyInternalAPI{
		DB:         db,
		ThisServer: cfg.Matrix.ServerName,
		FedClient:  fedClient,
		Producer:   keyChangeProducer,
	}
	updater := internal.NewDeviceListUpdater(db, ap, keyChangeProducer, fedClient, 8) // 8 workers TODO: configurable
	ap.Updater = updater
	go func() {
		if err := updater.Start(); err != nil {
			logrus.WithError(err).Panicf("failed to start device list updater")
		}
	}()

	keyconsumer := consumers.NewOutputCrossSigningKeyUpdateConsumer(
		base.ProcessContext, base.Cfg, consumer, db, ap,
	)
	if err := keyconsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start keyserver EDU server consumer")
	}

	return ap
}
