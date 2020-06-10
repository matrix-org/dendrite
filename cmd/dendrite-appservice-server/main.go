// Copyright 2018 Vector Creations Ltd
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
	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/internal/basecomponent"
)

func main() {
	cfg := basecomponent.ParseFlags(false)
	base := basecomponent.NewBaseDendrite(cfg, "AppServiceAPI", true)

	defer base.Close() // nolint: errcheck
	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()
	rsAPI := base.RoomserverHTTPClient()

	intAPI := appservice.NewInternalAPI(base, accountDB, deviceDB, rsAPI)
	appservice.AddInternalRoutes(base.InternalAPIMux, intAPI)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.AppServiceAPI), string(base.Cfg.Listen.AppServiceAPI))

}
