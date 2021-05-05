package pushserver

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/consumers"
	"github.com/matrix-org/dendrite/pushserver/internal"
	"github.com/matrix-org/dendrite/pushserver/inthttp"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/kafka"
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
	base *setup.BaseDendrite,
) api.PushserverInternalAPI {
	cfg := &base.Cfg.PushServer

	consumer, _ := kafka.SetupConsumerProducer(&cfg.Matrix.Kafka)

	pushserverDB, err := storage.Open(&cfg.Database)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to room server db")
	}

	psAPI := internal.NewPushserverAPI(
		cfg, pushserverDB,
	)

	rsConsumer := consumers.NewOutputRoomEventConsumer(
		base.ProcessContext, cfg, consumer, pushserverDB, psAPI,
	)
	if err := rsConsumer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start room server consumer")
	}

	return psAPI
}
