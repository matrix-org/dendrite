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
	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/syncapi"
)

func main() {
	cfg := basecomponent.ParseFlags()
	base := basecomponent.NewBaseDendrite(cfg, "SyncAPI")
	defer base.Close() // nolint: errcheck

	deviceDB := base.CreateDeviceDB()
	accountDB := base.CreateAccountsDB()

	_, _, query := base.CreateHTTPRoomserverAPIs()

	syncapi.SetupSyncAPIComponent(base, deviceDB, accountDB, query, nil)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.SyncAPI), string(base.Cfg.Listen.SyncAPI))

}
