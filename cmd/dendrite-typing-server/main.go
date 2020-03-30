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
	"github.com/matrix-org/dendrite/eduserver"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/sirupsen/logrus"
)

func main() {
	cfg := basecomponent.ParseFlags()
	base := basecomponent.NewBaseDendrite(cfg, "TypingServerAPI")
	defer func() {
		if err := base.Close(); err != nil {
			logrus.WithError(err).Warn("BaseDendrite close failed")
		}
	}()

	eduserver.SetupTypingServerComponent(base, cache.NewTypingCache())

	base.SetupAndServeHTTP(string(base.Cfg.Bind.TypingServer), string(base.Cfg.Listen.TypingServer))

}
