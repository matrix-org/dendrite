package internal

import (
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/federationsender/producers"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/gomatrixserverlib"
)

// FederationSenderInternalAPI is an implementation of api.FederationSenderInternalAPI
type FederationSenderInternalAPI struct {
	api.FederationSenderInternalAPI
	db         storage.Database
	cfg        *config.Dendrite
	statistics *types.Statistics
	producer   *producers.RoomserverProducer
	federation *gomatrixserverlib.FederationClient
	keyRing    *gomatrixserverlib.KeyRing
	queues     *queue.OutgoingQueues
}

func NewFederationSenderInternalAPI(
	db storage.Database, cfg *config.Dendrite,
	producer *producers.RoomserverProducer,
	federation *gomatrixserverlib.FederationClient,
	keyRing *gomatrixserverlib.KeyRing,
	statistics *types.Statistics,
	queues *queue.OutgoingQueues,
) *FederationSenderInternalAPI {
	return &FederationSenderInternalAPI{
		db:         db,
		cfg:        cfg,
		producer:   producer,
		federation: federation,
		keyRing:    keyRing,
		statistics: statistics,
		queues:     queues,
	}
}
