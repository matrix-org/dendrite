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
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/internal/transactions"
)

func main() {
	cfg := setup.ParseFlags(false)

	base := setup.NewBaseDendrite(cfg, "ClientAPI", true)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	federation := base.CreateFederationClient()

	asQuery := base.AppserviceHTTPClient()
	rsAPI := base.RoomserverHTTPClient()
	fsAPI := base.FederationSenderHTTPClient()
	eduInputAPI := base.EDUServerClient()
	userAPI := base.UserAPIClient()
	stateAPI := base.CurrentStateAPIClient()
	keyAPI := base.KeyServerHTTPClient()

	clientapi.AddPublicRoutes(
		base.PublicClientAPIMux, &base.Cfg.ClientAPI, base.KafkaProducer, accountDB, federation,
		rsAPI, eduInputAPI, asQuery, stateAPI, transactions.New(), fsAPI, userAPI, keyAPI, nil,
	)

	base.SetupAndServeHTTP(
		base.Cfg.ClientAPI.InternalAPI.Listen,
		base.Cfg.ClientAPI.ExternalAPI.Listen,
		nil, nil,
	)
}
