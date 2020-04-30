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
	_ "net/http/pprof"

	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/common/keydb"
	"github.com/matrix-org/dendrite/roomserver"
)

func main() {
	cfg := basecomponent.ParseFlags()
	base := basecomponent.NewBaseDendrite(cfg, "RoomServerAPI")
	defer base.Close() // nolint: errcheck
	keyDB := base.CreateKeyDB()
	federation := base.CreateFederationClient()
	keyRing := keydb.CreateKeyRing(federation.Client, keyDB, cfg.Matrix.KeyPerspectives)

	fsAPI := base.CreateHTTPFederationSenderAPIs()
	rsAPI := roomserver.SetupRoomServerComponent(base, keyRing, federation, nil) // TODO: AS API here
	rsAPI.SetFederationSenderAPI(fsAPI)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.RoomServer), string(base.Cfg.Listen.RoomServer))

}
