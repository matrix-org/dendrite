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
	"github.com/gorilla/mux"
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	eduserverAPI "github.com/matrix-org/dendrite/eduserver/api"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/config"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	serverkeyAPI "github.com/matrix-org/dendrite/serverkeyapi/api"

	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/gomatrixserverlib"
)

// AddPublicRoutes sets up and registers HTTP handlers on the base API muxes for the FederationAPI component.
func AddPublicRoutes(
	router *mux.Router,
	cfg *config.Dendrite,
	accountsDB accounts.Database,
	deviceDB devices.Database,
	federation *gomatrixserverlib.FederationClient,
	skAPI serverkeyAPI.ServerKeyInternalAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	asAPI appserviceAPI.AppServiceQueryAPI,
	federationSenderAPI federationSenderAPI.FederationSenderInternalAPI,
	eduAPI eduserverAPI.EDUServerInputAPI,
) {
	routing.Setup(
		router, cfg, rsAPI, asAPI,
		eduAPI, federationSenderAPI, skAPI,
		federation, accountsDB, deviceDB,
	)
}
