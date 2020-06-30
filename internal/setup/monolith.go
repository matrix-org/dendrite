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
	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi"
	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	eduServerAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationapi"
	federationSenderAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/transactions"
	"github.com/matrix-org/dendrite/keyserver"
	"github.com/matrix-org/dendrite/mediaapi"
	"github.com/matrix-org/dendrite/publicroomsapi"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/publicroomsapi/types"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	serverKeyAPI "github.com/matrix-org/dendrite/serverkeyapi/api"
	"github.com/matrix-org/dendrite/syncapi"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/dendrite/userapi/storage/devices"
	"github.com/matrix-org/gomatrixserverlib"
)

// Monolith represents an instantiation of all dependencies required to build
// all components of Dendrite, for use in monolith mode.
type Monolith struct {
	Config        *config.Dendrite
	DeviceDB      devices.Database
	AccountDB     accounts.Database
	KeyRing       *gomatrixserverlib.KeyRing
	Client        *gomatrixserverlib.Client
	FedClient     *gomatrixserverlib.FederationClient
	KafkaConsumer sarama.Consumer
	KafkaProducer sarama.SyncProducer

	AppserviceAPI       appserviceAPI.AppServiceQueryAPI
	EDUInternalAPI      eduServerAPI.EDUServerInputAPI
	FederationSenderAPI federationSenderAPI.FederationSenderInternalAPI
	RoomserverAPI       roomserverAPI.RoomserverInternalAPI
	ServerKeyAPI        serverKeyAPI.ServerKeyInternalAPI
	UserAPI             userapi.UserInternalAPI
	StateAPI            currentstateAPI.CurrentStateInternalAPI

	// TODO: can we remove this? It's weird that we are required the database
	// yet every other component can do that on its own. libp2p-demo uses a custom
	// database though annoyingly.
	PublicRoomsDB storage.Database

	// Optional
	ExtPublicRoomsProvider types.ExternalPublicRoomsProvider
}

// AddAllPublicRoutes attaches all public paths to the given router
func (m *Monolith) AddAllPublicRoutes(publicMux *mux.Router) {
	clientapi.AddPublicRoutes(
		publicMux, m.Config, m.KafkaProducer, m.DeviceDB, m.AccountDB,
		m.FedClient, m.RoomserverAPI,
		m.EDUInternalAPI, m.AppserviceAPI, m.StateAPI, transactions.New(),
		m.FederationSenderAPI, m.UserAPI,
	)

	keyserver.AddPublicRoutes(publicMux, m.Config, m.UserAPI)
	federationapi.AddPublicRoutes(
		publicMux, m.Config, m.UserAPI, m.FedClient,
		m.KeyRing, m.RoomserverAPI, m.FederationSenderAPI,
		m.EDUInternalAPI,
	)
	mediaapi.AddPublicRoutes(publicMux, m.Config, m.UserAPI, m.Client)
	publicroomsapi.AddPublicRoutes(
		publicMux, m.Config, m.KafkaConsumer, m.UserAPI, m.PublicRoomsDB, m.RoomserverAPI, m.FedClient,
		m.ExtPublicRoomsProvider,
	)
	syncapi.AddPublicRoutes(
		publicMux, m.KafkaConsumer, m.UserAPI, m.RoomserverAPI, m.FedClient, m.Config,
	)
}
