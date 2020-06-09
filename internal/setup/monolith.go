package setup

import (
	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/producers"
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
	"github.com/matrix-org/gomatrixserverlib"
)

// Monolith represents an instantiation of all dependencies required to build
// all components of Dendrite, for use in monolith mode.
type Monolith struct {
	Config        *config.Dendrite
	DeviceDB      devices.Database
	AccountDB     accounts.Database
	KeyRing       *gomatrixserverlib.KeyRing
	FedClient     *gomatrixserverlib.FederationClient
	KafkaConsumer sarama.Consumer
	KafkaProducer sarama.SyncProducer

	AppserviceAPI       appserviceAPI.AppServiceQueryAPI
	EDUInternalAPI      eduServerAPI.EDUServerInputAPI
	FederationSenderAPI federationSenderAPI.FederationSenderInternalAPI
	RoomserverAPI       roomserverAPI.RoomserverInternalAPI
	ServerKeyAPI        serverKeyAPI.ServerKeyInternalAPI

	// TODO: remove, this isn't even a producer
	EDUProducer *producers.EDUServerProducer
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
		publicMux, m.Config, m.KafkaConsumer, m.KafkaProducer, m.DeviceDB, m.AccountDB,
		m.FedClient, m.KeyRing, m.RoomserverAPI,
		m.EDUInternalAPI, m.AppserviceAPI, transactions.New(),
		m.FederationSenderAPI,
	)

	keyserver.AddPublicRoutes(publicMux, m.Config, m.DeviceDB, m.AccountDB)
	federationapi.AddPublicRoutes(
		publicMux, m.Config, m.AccountDB, m.DeviceDB, m.FedClient,
		m.KeyRing, m.RoomserverAPI, m.AppserviceAPI, m.FederationSenderAPI,
		m.EDUProducer,
	)
	mediaapi.AddPublicRoutes(publicMux, m.Config, m.DeviceDB)
	publicroomsapi.AddPublicRoutes(
		publicMux, m.Config, m.KafkaConsumer, m.DeviceDB, m.PublicRoomsDB, m.RoomserverAPI, m.FedClient,
		m.ExtPublicRoomsProvider,
	)
	syncapi.AddPublicRoutes(
		publicMux, m.KafkaConsumer, m.DeviceDB, m.AccountDB, m.RoomserverAPI, m.FedClient, m.Config,
	)
}
