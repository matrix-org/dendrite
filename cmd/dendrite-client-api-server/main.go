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

package main

import (
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/internal/transactions"
)

func main() {
	cfg := basecomponent.ParseFlags(false)

	base := basecomponent.NewBaseDendrite(cfg, "ClientAPI", true)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	federation := base.CreateFederationClient()

	serverKeyAPI := base.ServerKeyAPIClient()
	keyRing := serverKeyAPI.KeyRing()

	asQuery := base.AppserviceHTTPClient()
	rsAPI := base.RoomserverHTTPClient()
	fsAPI := base.FederationSenderHTTPClient()
	eduInputAPI := base.EDUServerClient()

	clientapi.AddPublicRoutes(
		base.PublicAPIMux, base, deviceDB, accountDB, federation, keyRing,
		rsAPI, eduInputAPI, asQuery, transactions.New(), fsAPI,
	)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.ClientAPI), string(base.Cfg.Listen.ClientAPI))

}
