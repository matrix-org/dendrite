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
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
)

func main() {
	cfg := basecomponent.ParseFlags()

	base := basecomponent.NewBaseDendrite(cfg, "ClientAPI")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	keyDB := base.CreateKeyDB()
	federation := base.CreateFederationClient()
	keyRing := keydb.CreateKeyRing(federation.Client, keyDB, cfg)

	asQuery := base.CreateHTTPAppServiceAPIs()
	alias, input, query := base.CreateHTTPRoomserverAPIs()
	fedSenderAPI := base.CreateHTTPFederationSenderAPIs()
	eduInputAPI := eduserver.SetupEDUServerComponent(base, cache.New())

	clientapi.SetupClientAPIComponent(
		base, deviceDB, accountDB, federation, &keyRing,
		alias, input, query, eduInputAPI, asQuery, transactions.New(), fedSenderAPI,
	)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.ClientAPI), string(base.Cfg.Listen.ClientAPI))

}
