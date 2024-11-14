// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package setup

import (
	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/clientapi"
	"github.com/element-hq/dendrite/clientapi/api"
	"github.com/element-hq/dendrite/federationapi"
	federationAPI "github.com/element-hq/dendrite/federationapi/api"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/httputil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/internal/transactions"
	"github.com/element-hq/dendrite/mediaapi"
	"github.com/element-hq/dendrite/relayapi"
	relayAPI "github.com/element-hq/dendrite/relayapi/api"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/setup/jetstream"
	"github.com/element-hq/dendrite/setup/process"
	"github.com/element-hq/dendrite/syncapi"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
)

// Monolith represents an instantiation of all dependencies required to build
// all components of Dendrite, for use in monolith mode.
type Monolith struct {
	Config    *config.Dendrite
	KeyRing   *gomatrixserverlib.KeyRing
	Client    *fclient.Client
	FedClient fclient.FederationClient

	AppserviceAPI appserviceAPI.AppServiceInternalAPI
	FederationAPI federationAPI.FederationInternalAPI
	RoomserverAPI roomserverAPI.RoomserverInternalAPI
	UserAPI       userapi.UserInternalAPI
	RelayAPI      relayAPI.RelayInternalAPI

	// Optional
	ExtPublicRoomsProvider   api.ExtraPublicRoomsProvider
	ExtUserDirectoryProvider userapi.QuerySearchProfilesAPI
}

// AddAllPublicRoutes attaches all public paths to the given router
func (m *Monolith) AddAllPublicRoutes(
	processCtx *process.ProcessContext,
	cfg *config.Dendrite,
	routers httputil.Routers,
	cm *sqlutil.Connections,
	natsInstance *jetstream.NATSInstance,
	caches *caching.Caches,
	enableMetrics bool,
) {
	userDirectoryProvider := m.ExtUserDirectoryProvider
	if userDirectoryProvider == nil {
		userDirectoryProvider = m.UserAPI
	}
	clientapi.AddPublicRoutes(
		processCtx, routers, cfg, natsInstance, m.FedClient, m.RoomserverAPI, m.AppserviceAPI, transactions.New(),
		m.FederationAPI, m.UserAPI, userDirectoryProvider,
		m.ExtPublicRoomsProvider, enableMetrics,
	)
	federationapi.AddPublicRoutes(
		processCtx, routers, cfg, natsInstance, m.UserAPI, m.FedClient, m.KeyRing, m.RoomserverAPI, m.FederationAPI, enableMetrics,
	)
	mediaapi.AddPublicRoutes(routers, cm, cfg, m.UserAPI, m.Client, m.FedClient, m.KeyRing)
	syncapi.AddPublicRoutes(processCtx, routers, cfg, cm, natsInstance, m.UserAPI, m.RoomserverAPI, caches, enableMetrics)

	if m.RelayAPI != nil {
		relayapi.AddPublicRoutes(routers, cfg, m.KeyRing, m.RelayAPI)
	}
}
