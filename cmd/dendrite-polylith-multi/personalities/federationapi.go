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

package personalities

import (
	"github.com/matrix-org/dendrite/federationapi"
	basepkg "github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
)

func FederationAPI(base *basepkg.BaseDendrite, cfg *config.Dendrite) {
	userAPI := base.UserAPIClient()
	federation := base.CreateFederationClient()
	rsAPI := base.RoomserverHTTPClient()
	keyAPI := base.KeyServerHTTPClient()
	fsAPI := federationapi.NewInternalAPI(base, federation, rsAPI, base.Caches, nil, true)
	keyRing := fsAPI.KeyRing()

	federationapi.AddPublicRoutes(
		base,
		userAPI, federation, keyRing,
		rsAPI, fsAPI, keyAPI, nil,
	)

	federationapi.AddInternalRoutes(base.InternalAPIMux, fsAPI)

	base.SetupAndServeHTTP(
		base.Cfg.FederationAPI.InternalAPI.Listen,
		base.Cfg.FederationAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
