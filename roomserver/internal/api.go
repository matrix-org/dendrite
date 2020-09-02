package internal

import (
	"context"
	"sync"

	"github.com/Shopify/sarama"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/perform"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomserverInternalAPI is an implementation of api.RoomserverInternalAPI
type RoomserverInternalAPI struct {
	DB                   storage.Database
	Cfg                  *config.RoomServer
	Producer             sarama.SyncProducer
	Cache                caching.RoomServerCaches
	ServerName           gomatrixserverlib.ServerName
	KeyRing              gomatrixserverlib.JSONVerifier
	FedClient            *gomatrixserverlib.FederationClient
	OutputRoomEventTopic string // Kafka topic for new output room events
	Inviter              *perform.Inviter
	Joiner               *perform.Joiner
	Leaver               *perform.Leaver
	Publisher            *perform.Publisher
	Backfiller           *perform.Backfiller
	mutexes              sync.Map // room ID -> *sync.Mutex, protects calls to processRoomEvent
	fsAPI                fsAPI.FederationSenderInternalAPI
}

func NewRoomserverAPI(
	cfg *config.RoomServer, roomserverDB storage.Database, producer sarama.SyncProducer,
	outputRoomEventTopic string, caches caching.RoomServerCaches, fedClient *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.JSONVerifier,
) *RoomserverInternalAPI {
	a := &RoomserverInternalAPI{
		DB:                   roomserverDB,
		Cfg:                  cfg,
		Producer:             producer,
		Cache:                caches,
		ServerName:           cfg.Matrix.ServerName,
		KeyRing:              keyRing,
		FedClient:            fedClient,
		OutputRoomEventTopic: outputRoomEventTopic,
		// perform-er structs get initialised when we have a federation sender to use
	}
	return a
}

// SetFederationSenderInputAPI passes in a federation sender input API reference
// so that we can avoid the chicken-and-egg problem of both the roomserver input API
// and the federation sender input API being interdependent.
func (r *RoomserverInternalAPI) SetFederationSenderAPI(fsAPI fsAPI.FederationSenderInternalAPI) {
	r.fsAPI = fsAPI

	r.Inviter = &perform.Inviter{
		DB:    r.DB,
		Cfg:   r.Cfg,
		FSAPI: r.fsAPI,
		RSAPI: r,
	}
	r.Joiner = &perform.Joiner{
		ServerName: r.Cfg.Matrix.ServerName,
		Cfg:        r.Cfg,
		DB:         r.DB,
		FSAPI:      r.fsAPI,
		RSAPI:      r,
	}
	r.Leaver = &perform.Leaver{
		Cfg:   r.Cfg,
		DB:    r.DB,
		FSAPI: r.fsAPI,
		RSAPI: r,
	}
	r.Publisher = &perform.Publisher{
		DB: r.DB,
	}
	r.Backfiller = &perform.Backfiller{
		ServerName: r.ServerName,
		DB:         r.DB,
		FedClient:  r.FedClient,
		KeyRing:    r.KeyRing,
	}
}

func (r *RoomserverInternalAPI) PerformInvite(
	ctx context.Context,
	req *api.PerformInviteRequest,
	res *api.PerformInviteResponse,
) error {
	outputEvents, err := r.Inviter.PerformInvite(ctx, req, res)
	if err != nil {
		return err
	}
	if len(outputEvents) == 0 {
		return nil
	}
	return r.WriteOutputEvents(req.Event.RoomID(), outputEvents)
}

func (r *RoomserverInternalAPI) PerformJoin(
	ctx context.Context,
	req *api.PerformJoinRequest,
	res *api.PerformJoinResponse,
) {
	r.Joiner.PerformJoin(ctx, req, res)
}

func (r *RoomserverInternalAPI) PerformLeave(
	ctx context.Context,
	req *api.PerformLeaveRequest,
	res *api.PerformLeaveResponse,
) error {
	outputEvents, err := r.Leaver.PerformLeave(ctx, req, res)
	if err != nil {
		return err
	}
	if len(outputEvents) == 0 {
		return nil
	}
	return r.WriteOutputEvents(req.RoomID, outputEvents)
}

func (r *RoomserverInternalAPI) PerformPublish(
	ctx context.Context,
	req *api.PerformPublishRequest,
	res *api.PerformPublishResponse,
) {
	r.Publisher.PerformPublish(ctx, req, res)
}

// Query a given amount (or less) of events prior to a given set of events.
func (r *RoomserverInternalAPI) PerformBackfill(
	ctx context.Context,
	request *api.PerformBackfillRequest,
	response *api.PerformBackfillResponse,
) error {
	return r.Backfiller.PerformBackfill(ctx, request, response)
}
