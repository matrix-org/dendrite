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
	"flag"
	"os"
	"strings"

	"github.com/matrix-org/dendrite/cmd/dendrite-polylith-multi/personalities"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"

	_ "github.com/mattn/go-sqlite3"
)

type entrypoint func(base *base.BaseDendrite, cfg *config.Dendrite)

func main() {
	cfg := setup.ParseFlags(true)

	component := ""
	if flag.NFlag() > 0 {
		component = flag.Arg(0) // ./dendrite-polylith-multi --config=... clientapi
	} else if len(os.Args) > 1 {
		component = os.Args[1] // ./dendrite-polylith-multi clientapi
	}

	components := map[string]entrypoint{
		"appservice":    personalities.Appservice,
		"clientapi":     personalities.ClientAPI,
		"federationapi": personalities.FederationAPI,
		"keyserver":     personalities.KeyServer,
		"mediaapi":      personalities.MediaAPI,
		"roomserver":    personalities.RoomServer,
		"syncapi":       personalities.SyncAPI,
		"userapi":       personalities.UserAPI,
	}

	start, ok := components[component]
	if !ok {
		if component == "" {
			logrus.Errorf("No component specified")
			logrus.Info("The first argument on the command line must be the name of the component to run")
		} else {
			logrus.Errorf("Unknown component %q specified", component)
		}

		var list []string
		for c := range components {
			list = append(list, c)
		}
		logrus.Infof("Valid components: %s", strings.Join(list, ", "))

		os.Exit(1)
	}

	logrus.Infof("Starting %q component", component)

	base := base.NewBaseDendrite(cfg, component) // TODO
	defer base.Close()                           // nolint: errcheck

	go start(base, cfg)
	base.WaitForShutdown()
}
