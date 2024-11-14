// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

// Package mscs implements Matrix Spec Changes from https://github.com/matrix-org/matrix-doc
package mscs

import (
	"context"

	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/mscs/msc2836"
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
