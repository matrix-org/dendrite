package internal

import (
	"sync"

	"github.com/Shopify/sarama"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomserverInternalAPI is an implementation of api.RoomserverInternalAPI
type RoomserverInternalAPI struct {
	DB                   storage.Database
	Cfg                  *config.RoomServer
	Producer             sarama.SyncProducer
	Cache                caching.RoomVersionCache
	ServerName           gomatrixserverlib.ServerName
	KeyRing              gomatrixserverlib.JSONVerifier
	FedClient            *gomatrixserverlib.FederationClient
	OutputRoomEventTopic string     // Kafka topic for new output room events
	mutex                sync.Mutex // Protects calls to processRoomEvent
	fsAPI                fsAPI.FederationSenderInternalAPI
}
