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
	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/internal"
	"github.com/matrix-org/dendrite/keyserver/inthttp"
	"github.com/matrix-org/dendrite/keyserver/producers"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
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
	cfg *config.KeyServer, fedClient *gomatrixserverlib.FederationClient, producer sarama.SyncProducer,
) api.KeyInternalAPI {
	db, err := storage.NewDatabase(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to key server database")
	}
	keyChangeProducer := &producers.KeyChange{
		Topic:    string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputKeyChangeEvent)),
		Producer: producer,
		DB:       db,
	}
	updater := internal.NewDeviceListUpdater(db, keyChangeProducer, fedClient, 8) // 8 workers TODO: configurable
	err = updater.Start()
	if err != nil {
		logrus.WithError(err).Panicf("failed to start device list updater")
	}
	return &internal.KeyInternalAPI{
		DB:         db,
		ThisServer: cfg.Matrix.ServerName,
		FedClient:  fedClient,
		Producer:   keyChangeProducer,
		Updater:    updater,
	}
}
