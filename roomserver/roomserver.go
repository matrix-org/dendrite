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
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/inthttp"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/roomserver/internal"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/sirupsen/logrus"
)

// SetupRoomServerComponent sets up and registers HTTP handlers for the
// RoomServer component. Returns instances of the various roomserver APIs,
// allowing other components running in the same process to hit the query the
// APIs directly instead of having to use HTTP.
func SetupRoomServerComponent(
	base *basecomponent.BaseDendrite,
	keyRing gomatrixserverlib.JSONVerifier,
	fedClient *gomatrixserverlib.FederationClient,
) api.RoomserverInternalAPI {
	roomserverDB, err := storage.Open(string(base.Cfg.Database.RoomServer), base.Cfg.DbProperties())
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to room server db")
	}

	internalAPI := &internal.RoomserverInternalAPI{
		DB:                   roomserverDB,
		Cfg:                  base.Cfg,
		Producer:             base.KafkaProducer,
		OutputRoomEventTopic: string(base.Cfg.Kafka.Topics.OutputRoomEvent),
		ImmutableCache:       base.ImmutableCache,
		ServerName:           base.Cfg.Matrix.ServerName,
		FedClient:            fedClient,
		KeyRing:              keyRing,
	}

	inthttp.AddRoutes(internalAPI, base.InternalAPIMux)

	return internalAPI
}
