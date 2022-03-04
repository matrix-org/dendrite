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
	"github.com/gorilla/mux"
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/clientapi/api"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationapi"
	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/transactions"
	keyAPI "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/mediaapi"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	userdb "github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

// Monolith represents an instantiation of all dependencies required to build
// all components of Dendrite, for use in monolith mode.
type Monolith struct {
	Config    *config.Dendrite
	AccountDB userdb.Database
	KeyRing   *gomatrixserverlib.KeyRing
	Client    *gomatrixserverlib.Client
	FedClient *gomatrixserverlib.FederationClient

	AppserviceAPI  appserviceAPI.AppServiceQueryAPI
	EDUInternalAPI eduServerAPI.EDUServerInputAPI
	FederationAPI  federationAPI.FederationInternalAPI
	RoomserverAPI  roomserverAPI.RoomserverInternalAPI
	UserAPI        userapi.UserInternalAPI
	KeyAPI         keyAPI.KeyInternalAPI

	// Optional
	ExtPublicRoomsProvider api.ExtraPublicRoomsProvider
}

// AddAllPublicRoutes attaches all public paths to the given router
func (m *Monolith) AddAllPublicRoutes(process *process.ProcessContext, csMux, ssMux, keyMux, wkMux, mediaMux, synapseMux *mux.Router) {
	clientapi.AddPublicRoutes(
		csMux, synapseMux, &m.Config.ClientAPI, m.AccountDB,
		m.FedClient, m.RoomserverAPI,
		m.EDUInternalAPI, m.AppserviceAPI, transactions.New(),
		m.FederationAPI, m.UserAPI, m.KeyAPI,
		m.ExtPublicRoomsProvider, &m.Config.MSCs,
	)
	federationapi.AddPublicRoutes(
		ssMux, keyMux, wkMux, &m.Config.FederationAPI, m.UserAPI, m.FedClient,
		m.KeyRing, m.RoomserverAPI, m.FederationAPI,
		m.EDUInternalAPI, m.KeyAPI, &m.Config.MSCs, nil,
	)
	mediaapi.AddPublicRoutes(mediaMux, &m.Config.MediaAPI, &m.Config.ClientAPI.RateLimiting, m.UserAPI, m.Client)
	syncapi.AddPublicRoutes(
		process, csMux, m.UserAPI, m.RoomserverAPI,
		m.KeyAPI, m.FedClient, &m.Config.SyncAPI,
	)
}
