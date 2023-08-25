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

// Package mscs implements Matrix Spec Changes from https://github.com/matrix-org/matrix-doc
package mscs

import (
	"context"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/mscs/msc2836"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// Enable MSCs - returns an error on unknown MSCs
func Enable(cfg *config.Dendrite, cm *sqlutil.Connections, routers httputil.Routers, monolith *setup.Monolith, caches *caching.Caches) error {
	for _, msc := range cfg.MSCs.MSCs {
		util.GetLogger(context.Background()).WithField("msc", msc).Info("Enabling MSC")
		if err := EnableMSC(cfg, cm, routers, monolith, msc, caches); err != nil {
			return err
		}
	}
	return nil
}

func EnableMSC(cfg *config.Dendrite, cm *sqlutil.Connections, routers httputil.Routers, monolith *setup.Monolith, msc string, caches *caching.Caches) error {
	switch msc {
	case "msc2836":
		return msc2836.Enable(cfg, cm, routers, monolith.RoomserverAPI, monolith.FederationAPI, monolith.UserAPI, monolith.KeyRing)
	case "msc2444": // enabled inside federationapi
	case "msc2753": // enabled inside clientapi
	default:
		logrus.Warnf("EnableMSC: unknown MSC '%s', this MSC is either not supported or is natively supported by Dendrite", msc)
	}
	return nil
}
