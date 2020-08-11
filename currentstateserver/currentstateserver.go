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

package currentstateserver

import (
	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/currentstateserver/acls"
	"github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/currentstateserver/consumers"
	"github.com/matrix-org/dendrite/currentstateserver/internal"
	"github.com/matrix-org/dendrite/currentstateserver/inthttp"
	"github.com/matrix-org/dendrite/currentstateserver/storage"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.CurrentStateInternalAPI) {
	inthttp.AddRoutes(router, intAPI)
}

// NewInternalAPI returns a concrete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(cfg *config.CurrentStateServer, consumer sarama.Consumer) api.CurrentStateInternalAPI {
	csDB, err := storage.NewDatabase(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to open database")
	}
	serverACLs := acls.NewServerACLs(csDB)
	roomConsumer := consumers.NewOutputRoomEventConsumer(
		cfg.Matrix.Kafka.TopicFor(config.TopicOutputRoomEvent), consumer, csDB, serverACLs,
	)
	if err = roomConsumer.Start(); err != nil {
		logrus.WithError(err).Panicf("failed to start room server consumer")
	}
	return &internal.CurrentStateInternalAPI{
		DB:         csDB,
		ServerACLs: serverACLs,
	}
}
