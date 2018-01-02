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
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/federationsender/consumers"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// SetupFederationSenderComponent sets up and registers HTTP handlers for the
// FederationSender component.
func SetupFederationSenderComponent(
	base *basecomponent.BaseDendrite,
	federation *gomatrixserverlib.FederationClient,
	queryAPI api.RoomserverQueryAPI,
) {
	federationSenderDB, err := storage.NewDatabase(string(base.Cfg.Database.FederationSender))
	if err != nil {
		logrus.WithError(err).Panic("failed to connect to federation sender db")
	}

	queues := queue.NewOutgoingQueues(base.Cfg.Matrix.ServerName, federation)

	consumer := consumers.NewOutputRoomEventConsumer(
		base.Cfg, base.KafkaConsumer, queues,
		federationSenderDB, queryAPI,
	)
	if err = consumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start room server consumer")
	}
}
