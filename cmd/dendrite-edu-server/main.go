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

	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/sirupsen/logrus"
)

func main() {
	cfg := setup.ParseFlags(false)
	base := setup.NewBaseDendrite(cfg, "EDUServerAPI", true)
	defer func() {
		if err := base.Close(); err != nil {
			logrus.WithError(err).Warn("BaseDendrite close failed")
		}
	}()

	intAPI := eduserver.NewInternalAPI(base, cache.New(), base.UserAPIClient())
	eduserver.AddInternalRoutes(base.InternalAPIMux, intAPI)

	base.SetupAndServeHTTP(
		base.Cfg.EDUServer.InternalAPI.Listen,
		setup.NoExternalListener,
		nil, nil,
	)
}
