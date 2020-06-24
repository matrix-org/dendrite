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

package roomserver

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/inthttp"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/roomserver/internal"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.RoomserverInternalAPI) {
	inthttp.AddRoutes(intAPI, router)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *setup.BaseDendrite,
	keyRing gomatrixserverlib.JSONVerifier,
	fedClient *gomatrixserverlib.FederationClient,
) api.RoomserverInternalAPI {
	roomserverDB, err := storage.Open(string(base.Cfg.Database.RoomServer), base.Cfg.DbProperties())
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to room server db")
	}

	return &internal.RoomserverInternalAPI{
		DB:                   roomserverDB,
		Cfg:                  base.Cfg,
		Producer:             base.KafkaProducer,
		OutputRoomEventTopic: string(base.Cfg.Kafka.Topics.OutputRoomEvent),
		Cache:                base.Caches,
		ServerName:           base.Cfg.Matrix.ServerName,
		FedClient:            fedClient,
		KeyRing:              keyRing,
	}
}
