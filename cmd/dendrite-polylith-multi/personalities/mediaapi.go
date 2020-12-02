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
	"github.com/matrix-org/dendrite/mediaapi"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
)

func MediaAPI(base *setup.BaseDendrite, cfg *config.Dendrite) {
	userAPI := base.UserAPIClient()
	client := base.CreateClient()

	mediaapi.AddPublicRoutes(base.PublicMediaAPIMux, &base.Cfg.MediaAPI, userAPI, client)

	base.SetupAndServeHTTP(
		base.Cfg.MediaAPI.InternalAPI.Listen,
		base.Cfg.MediaAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
