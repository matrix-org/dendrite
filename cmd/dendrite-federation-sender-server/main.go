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
	"github.com/matrix-org/dendrite/federationsender"
)

func main() {
	cfg := basecomponent.ParseFlags()
	base := basecomponent.NewBaseDendrite(cfg, "FederationSender")
	defer base.Close() // nolint: errcheck

	federation := base.CreateFederationClient()

	_, input, query := base.CreateHTTPRoomserverAPIs()

	federationsender.SetupFederationSenderComponent(
		base, federation, query, input,
	)

	base.SetupAndServeHTTP(string(base.Cfg.Bind.FederationSender), string(base.Cfg.Listen.FederationSender))

}
