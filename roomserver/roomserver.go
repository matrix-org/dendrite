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
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/base"
)

// NewInternalAPI returns a concrete implementation of the internal API.
func NewInternalAPI(
	base *base.BaseDendrite,
) api.RoomserverInternalAPI {
	cfg := &base.Cfg.RoomServer

	roomserverDB, err := storage.Open(base, &cfg.Database, base.Caches)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to room server db")
	}

	js, nc := base.NATS.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)

	return internal.NewRoomserverAPI(
		base, roomserverDB, js, nc,
	)
}
