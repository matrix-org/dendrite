package pushserver

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/internal"
	"github.com/matrix-org/dendrite/pushserver/inthttp"
	"github.com/matrix-org/dendrite/pushserver/producers"
	"github.com/matrix-org/dendrite/pushserver/storage"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/kafka"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/sirupsen/logrus"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.PushserverInternalAPI) {
	inthttp.AddRoutes(intAPI, router)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	cfg *config.PushServer,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	userAPI uapi.UserInternalAPI,
) api.PushserverInternalAPI {
	db, err := storage.Open(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to push server db")
	}

	_, producer := kafka.SetupConsumerProducer(&cfg.Matrix.Kafka)
	syncProducer := &producers.SyncAPIProducer{
		Producer: producer,
		// TODO: user API should handle syncs for account data. Right now,
		// it's handled by clientapi, and hence uses its topic. When user
		// API handles it for all account data, we can remove it from
		// here.
		ClientDataTopic: cfg.Matrix.Kafka.TopicFor(config.TopicOutputClientData),
	}

	psAPI := internal.NewPushserverAPI(
		cfg, db, userAPI, syncProducer,
	)

	return psAPI
}
