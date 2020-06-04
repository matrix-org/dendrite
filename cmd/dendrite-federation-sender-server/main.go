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
	"github.com/matrix-org/dendrite/federationsender"
	"github.com/matrix-org/dendrite/internal/basecomponent"
)

func main() {
	cfg := basecomponent.ParseFlags()
	base := basecomponent.NewBaseDendrite(cfg, "FederationSender", true)
	defer base.Close() // nolint: errcheck

	federation := base.CreateFederationClient()

	serverKeyAPI := base.ServerKeyAPIClient()
	keyRing := serverKeyAPI.KeyRing()

	rsAPI := base.RoomserverHTTPClient()
	fsAPI := federationsender.SetupFederationSenderComponent(
		base, federation, rsAPI, keyRing,
	)
	rsAPI.SetFederationSenderAPI(fsAPI)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.FederationSender), string(base.Cfg.Listen.FederationSender))

}
