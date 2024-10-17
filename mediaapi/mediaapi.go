// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only
// Please see LICENSE in the repository root for full details.

package mediaapi

import (
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/mediaapi/routing"
	"github.com/matrix-org/dendrite/mediaapi/storage"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/sirupsen/logrus"
)

// AddPublicRoutes sets up and registers HTTP handlers for the MediaAPI component.
func AddPublicRoutes(
	routers httputil.Routers,
	cm *sqlutil.Connections,
	cfg *config.Dendrite,
	userAPI userapi.MediaUserAPI,
	client *fclient.Client,
	fedClient fclient.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier,
) {
	mediaDB, err := storage.NewMediaAPIDatasource(cm, &cfg.MediaAPI.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to media db")
	}

	routing.Setup(
		routers, cfg, mediaDB, userAPI, client, fedClient, keyRing,
	)
}
