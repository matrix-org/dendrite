// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package relayapi_test

import (
	"testing"

	"github.com/matrix-org/dendrite/relayapi"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/stretchr/testify/assert"
)

func TestCreateNewRelayInternalAPI(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, close := testrig.CreateBaseDendrite(t, dbType)
		base.Cfg.FederationAPI.PreferDirectFetch = true
		base.Cfg.FederationAPI.KeyPerspectives = nil
		defer close()
		jsctx, _ := base.NATS.Prepare(base.ProcessContext, &base.Cfg.Global.JetStream)
		defer jetstream.DeleteAllStreams(jsctx, &base.Cfg.Global.JetStream)

		relayAPI := relayapi.NewRelayInternalAPI(
			base,
			nil,
			nil,
			nil,
			nil,
		)
		assert.NotNil(t, relayAPI)
	})
}
