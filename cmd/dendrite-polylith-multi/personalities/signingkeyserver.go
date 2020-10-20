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
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/signingkeyserver"
)

func SigningKeyServer(base *setup.BaseDendrite, cfg *config.Dendrite) {
	federation := base.CreateFederationClient()

	intAPI := signingkeyserver.NewInternalAPI(&base.Cfg.SigningKeyServer, federation, base.Caches)
	signingkeyserver.AddInternalRoutes(base.InternalAPIMux, intAPI, base.Caches)

	base.SetupAndServeHTTP(
		base.Cfg.SigningKeyServer.InternalAPI.Listen,
		setup.NoListener,
		nil, nil,
	)
}
