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
	"net/http"

	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/roomserver/alias"
	"github.com/matrix-org/dendrite/roomserver/input"
	"github.com/matrix-org/dendrite/roomserver/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/sirupsen/logrus"
)

// SetupRoomServerComponent sets up and registers HTTP handlers for the RoomServer
// component.
func SetupRoomServerComponent(
	base *basecomponent.BaseDendrite,
) {
	roomserverDB, err := storage.Open(string(base.Cfg.Database.RoomServer))
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to room server db")
	}

	SetupRoomServerComponentWithDB(base, roomserverDB)
}

// SetupRoomServerComponentWithDB sets up and registers HTTP handlers for the RoomServer
// component, reusing the given room server database instance.
func SetupRoomServerComponentWithDB(
	base *basecomponent.BaseDendrite, roomserverDB *storage.Database,
) {
	inputAPI := input.RoomserverInputAPI{
		DB:                   roomserverDB,
		Producer:             base.KafkaProducer,
		OutputRoomEventTopic: string(base.Cfg.Kafka.Topics.OutputRoomEvent),
	}

	inputAPI.SetupHTTP(http.DefaultServeMux)

	queryAPI := query.RoomserverQueryAPI{DB: roomserverDB}

	queryAPI.SetupHTTP(http.DefaultServeMux)

	aliasAPI := alias.RoomserverAliasAPI{
		DB:       roomserverDB,
		Cfg:      base.Cfg,
		InputAPI: &inputAPI,
		QueryAPI: &queryAPI,
	}

	aliasAPI.SetupHTTP(http.DefaultServeMux)
}
