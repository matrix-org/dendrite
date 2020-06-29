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

package federationsender

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/consumers"
	"github.com/matrix-org/dendrite/federationsender/internal"
	"github.com/matrix-org/dendrite/federationsender/inthttp"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/setup"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.FederationSenderInternalAPI) {
	inthttp.AddRoutes(intAPI, router)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *setup.BaseDendrite,
	federation *gomatrixserverlib.FederationClient,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	keyRing *gomatrixserverlib.KeyRing,
) api.FederationSenderInternalAPI {
	federationSenderDB, err := storage.NewDatabase(string(base.Cfg.Database.FederationSender), base.Cfg.DbProperties())
	if err != nil {
		logrus.WithError(err).Panic("failed to connect to federation sender db")
	}

	statistics := &types.Statistics{}
	queues := queue.NewOutgoingQueues(
		federationSenderDB, base.Cfg.Matrix.ServerName, federation, rsAPI, statistics,
		&queue.SigningInfo{
			KeyID:      base.Cfg.Matrix.KeyID,
			PrivateKey: base.Cfg.Matrix.PrivateKey,
			ServerName: base.Cfg.Matrix.ServerName,
		},
	)

	rsConsumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, queues,
		federationSenderDB, rsAPI,
	)
	if err = rsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start room server consumer")
	}

	tsConsumer := consumers.NewOutputTypingEventConsumer(
		base.Cfg, base.KafkaConsumer, queues, federationSenderDB,
	)
	if err := tsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start typing server consumer")
	}

	return internal.NewFederationSenderInternalAPI(federationSenderDB, base.Cfg, rsAPI, federation, keyRing, statistics, queues)
}
