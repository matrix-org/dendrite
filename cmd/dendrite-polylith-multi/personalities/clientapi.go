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
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/internal/transactions"
	basepkg "github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
)

func ClientAPI(base *basepkg.BaseDendrite, cfg *config.Dendrite) {
	accountDB := base.CreateAccountsDB()
	federation := base.CreateFederationClient()

	asQuery := base.AppserviceHTTPClient()
	rsAPI := base.RoomserverHTTPClient()
	fsAPI := base.FederationAPIHTTPClient()
	eduInputAPI := base.EDUServerClient()
	userAPI := base.UserAPIClient()
	keyAPI := base.KeyServerHTTPClient()

	clientapi.AddPublicRoutes(
		base.PublicClientAPIMux, base.SynapseAdminMux, &base.Cfg.ClientAPI, accountDB, federation,
		rsAPI, eduInputAPI, asQuery, transactions.New(), fsAPI, userAPI, keyAPI, nil,
		&cfg.MSCs,
	)

	base.SetupAndServeHTTP(
		base.Cfg.ClientAPI.InternalAPI.Listen,
		base.Cfg.ClientAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
