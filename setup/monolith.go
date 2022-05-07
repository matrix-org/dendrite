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

package setup

import (
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/clientapi/api"
	"github.com/matrix-org/dendrite/federationapi"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/transactions"
	keyAPI "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/mediaapi"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/syncapi"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// Monolith represents an instantiation of all dependencies required to build
// all components of Dendrite, for use in monolith mode.
type Monolith struct {
	Config    *config.Dendrite
	KeyRing   *gomatrixserverlib.KeyRing
	Client    *gomatrixserverlib.Client
	FedClient *gomatrixserverlib.FederationClient

	AppserviceAPI appserviceAPI.AppServiceInternalAPI
	FederationAPI federationAPI.FederationInternalAPI
	RoomserverAPI roomserverAPI.RoomserverInternalAPI
	UserAPI       userapi.UserInternalAPI
	KeyAPI        keyAPI.KeyInternalAPI

	// Optional
	ExtPublicRoomsProvider   api.ExtraPublicRoomsProvider
	ExtUserDirectoryProvider userapi.QuerySearchProfilesAPI
}

// AddAllPublicRoutes attaches all public paths to the given router
func (m *Monolith) AddAllPublicRoutes(base *base.BaseDendrite) {
	userDirectoryProvider := m.ExtUserDirectoryProvider
	if userDirectoryProvider == nil {
		userDirectoryProvider = m.UserAPI
	}
	clientapi.AddPublicRoutes(
		base, m.FedClient, m.RoomserverAPI, m.AppserviceAPI, transactions.New(),
		m.FederationAPI, m.UserAPI, userDirectoryProvider, m.KeyAPI,
		m.ExtPublicRoomsProvider,
	)
	federationapi.AddPublicRoutes(
		base, m.UserAPI, m.FedClient, m.KeyRing, m.RoomserverAPI, m.FederationAPI,
		m.KeyAPI, nil,
	)
	mediaapi.AddPublicRoutes(
		base, m.UserAPI, m.Client,
	)
	syncapi.AddPublicRoutes(
		base, m.UserAPI, m.RoomserverAPI, m.KeyAPI,
	)
}
