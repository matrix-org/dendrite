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

package publicroomsapi

import (
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/publicroomsapi/routing"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/sirupsen/logrus"
)

// SetupPublicRoomsAPIComponent sets up and registers HTTP handlers for the PublicRoomsAPI
// component.
func SetupPublicRoomsAPIComponent(
	base *basecomponent.BaseDendrite,
	tracers *common.Tracers,
	deviceDB *devices.Database,
) {
	publicRoomsDB, err := storage.NewPublicRoomsServerDatabase(tracers, string(base.Cfg.Database.PublicRoomsAPI))
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to public rooms db")
	}

	tracer := tracers.SetupNewTracer("Dendrite - PublicRoomsAPI")

	routing.Setup(base.APIMux, deviceDB, publicRoomsDB, tracer)
}
