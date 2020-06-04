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

package main

import (
	"github.com/matrix-org/dendrite/internal/basecomponent"
	"github.com/matrix-org/dendrite/keyserver"
)

func main() {
	cfg := basecomponent.ParseFlags(false)
	base := basecomponent.NewBaseDendrite(cfg, "KeyServer", true)
	defer base.Close() // nolint: errcheck

	accountDB := base.CreateAccountsDB()
	deviceDB := base.CreateDeviceDB()

	keyserver.SetupKeyServerComponent(base, deviceDB, accountDB)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.KeyServer), string(base.Cfg.Listen.KeyServer))

}
