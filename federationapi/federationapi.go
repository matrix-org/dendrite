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
	eduserverAPI "github.com/matrix-org/dendrite/eduserver/api"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	keyserverAPI "github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/gomatrixserverlib"
)

// AddPublicRoutes sets up and registers HTTP handlers on the base API muxes for the FederationAPI component.
func AddPublicRoutes(
	fedRouter, keyRouter *mux.Router,
	cfg *config.FederationAPI,
	userAPI userapi.UserInternalAPI,
	federation *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	federationSenderAPI federationSenderAPI.FederationSenderInternalAPI,
	eduAPI eduserverAPI.EDUServerInputAPI,
	keyAPI keyserverAPI.KeyInternalAPI,
	mscCfg *config.MSCs,
) {
	routing.Setup(
		fedRouter, keyRouter, cfg, rsAPI,
		eduAPI, federationSenderAPI, keyRing,
		federation, userAPI, keyAPI, mscCfg,
	)
}
