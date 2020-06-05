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
	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/internal/setup"
)

func main() {
	cfg := setup.ParseFlags(false)
	base := setup.NewBase(cfg, "FederationAPI", true)
	defer base.Close() // nolint: errcheck
	// TODO: this isn't a producer
	eduProducer := producers.NewEDUServerProducer(base.EDUServer())

	federationapi.SetupFederationAPIComponent(base, eduProducer)
	base.SetupAndServeHTTP(string(base.Cfg.Bind.FederationAPI), string(base.Cfg.Listen.FederationAPI))

}
