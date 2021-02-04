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
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi"
)

func SyncAPI(base *setup.BaseDendrite, cfg *config.Dendrite) {
	userAPI := base.UserAPIClient()
	federation := base.CreateFederationClient()

	rsAPI := base.RoomserverHTTPClient()

	syncapi.AddPublicRoutes(
		base.ProcessContext,
		base.PublicClientAPIMux, userAPI, rsAPI,
		base.KeyServerHTTPClient(),
		federation, &cfg.SyncAPI,
	)

	base.SetupAndServeHTTP(
		base.Cfg.SyncAPI.InternalAPI.Listen,
		base.Cfg.SyncAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
