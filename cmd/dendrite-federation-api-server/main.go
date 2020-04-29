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
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/federationapi"
)

func main() {
	cfg := basecomponent.ParseFlags()
	base := basecomponent.NewBaseDendrite(cfg, "FederationAPI")
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	keyDB := base.CreateKeyDB()
	federation := base.CreateFederationClient()
	fsAPI := base.CreateHTTPFederationSenderAPIs()
	keyRing := keydb.CreateKeyRing(federation.Client, keyDB, cfg.Matrix.KeyPerspectives)

	alias, input, query := base.CreateHTTPRoomserverAPIs()
	asQuery := base.CreateHTTPAppServiceAPIs()
	eduInputAPI := eduserver.SetupEDUServerComponent(base, cache.New())
	eduProducer := producers.NewEDUServerProducer(eduInputAPI)

	federationapi.SetupFederationAPIComponent(
		base, accountDB, deviceDB, federation, &keyRing,
		alias, input, query, asQuery, fsAPI, eduProducer,
	)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.FederationAPI), string(base.Cfg.Listen.FederationAPI))

}
