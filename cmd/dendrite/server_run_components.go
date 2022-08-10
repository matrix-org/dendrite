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
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/matrix-org/dendrite/cmd/dendrite/components"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
)

type entrypoint func(base *base.BaseDendrite, cfg *config.Dendrite)

var allComponents = map[string]entrypoint{
	"appservice":    components.Appservice,
	"clientapi":     components.ClientAPI,
	"federationapi": components.FederationAPI,
	"keyserver":     components.KeyServer,
	"mediaapi":      components.MediaAPI,
	"roomserver":    components.RoomServer,
	"syncapi":       components.SyncAPI,
	"userapi":       components.UserAPI,
}

func init() {
	for component, start := range allComponents {
		runServerCmd.AddCommand(&cobra.Command{
			Use:    component,
			Short:  fmt.Sprintf("Run dendrite server polylith component: %s", component),
			PreRun: readConfigCmd(false),
			Run: func(cmd *cobra.Command, args []string) {
				logrus.Infof("Starting %q component", component)

				base := base.NewBaseDendrite(cfg, component, base.PolylithMode) // TODO
				defer base.Close()                                              // nolint: errcheck

				go start(base, cfg)
				base.WaitForShutdown()
			},
		})
	}
}
