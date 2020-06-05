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

package federationapi

import (
	"github.com/matrix-org/dendrite/internal/setup"

	// TODO: Are we really wanting to pull in the producer from clientapi
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/federationapi/routing"
)

// SetupFederationAPIComponent sets up and registers HTTP handlers for the
// FederationAPI component.
func SetupFederationAPIComponent(
	base *setup.Base,
	eduProducer *producers.EDUServerProducer,
) {
	roomserverProducer := producers.NewRoomserverProducer(base.RoomserverAPI())

	routing.Setup(
		base.PublicAPIMux, base.Cfg, base.RoomserverAPI(), base.AppserviceAPI(), roomserverProducer,
		eduProducer, base.FederationSender(), *base.ServerKeyAPI().KeyRing(),
		base.FederationClient, base.AccountDB, base.DeviceDB,
	)
}
