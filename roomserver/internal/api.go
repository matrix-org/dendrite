package internal

import (
	"context"

	"github.com/Shopify/sarama"
	asAPI "github.com/matrix-org/dendrite/appservice/api"
	fsAPI "github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/internal/perform"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomserverInternalAPI is an implementation of api.RoomserverInternalAPI
type RoomserverInternalAPI struct {
	*input.Inputer
	*query.Queryer
	*perform.Inviter
	*perform.Joiner
	*perform.Peeker
	*perform.InboundPeeker
	*perform.Unpeeker
	*perform.Leaver
	*perform.Publisher
	*perform.Backfiller
	*perform.Forgetter
	DB                     storage.Database
	Cfg                    *config.RoomServer
	Producer               sarama.SyncProducer
	Cache                  caching.RoomServerCaches
	ServerName             gomatrixserverlib.ServerName
	KeyRing                gomatrixserverlib.JSONVerifier
	fsAPI                  fsAPI.FederationSenderInternalAPI
	asAPI                  asAPI.AppServiceQueryAPI
	OutputRoomEventTopic   string // Kafka topic for new output room events
	PerspectiveServerNames []gomatrixserverlib.ServerName
}

func NewRoomserverAPI(
	cfg *config.RoomServer, roomserverDB storage.Database, producer sarama.SyncProducer,
	outputRoomEventTopic string, caches caching.RoomServerCaches,
	keyRing gomatrixserverlib.JSONVerifier, perspectiveServerNames []gomatrixserverlib.ServerName,
) *RoomserverInternalAPI {
	serverACLs := acls.NewServerACLs(roomserverDB)
	a := &RoomserverInternalAPI{
		DB:                     roomserverDB,
		Cfg:                    cfg,
		Cache:                  caches,
		ServerName:             cfg.Matrix.ServerName,
		PerspectiveServerNames: perspectiveServerNames,
		KeyRing:                keyRing,
		Queryer: &query.Queryer{
			DB:         roomserverDB,
			Cache:      caches,
			ServerACLs: serverACLs,
		},
		Inputer: &input.Inputer{
			DB:                   roomserverDB,
			OutputRoomEventTopic: outputRoomEventTopic,
			Producer:             producer,
			ServerName:           cfg.Matrix.ServerName,
			ACLs:                 serverACLs,
		},
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
		DB:      r.DB,
		Cfg:     r.Cfg,
		FSAPI:   r.fsAPI,
		Inputer: r.Inputer,
	}
	r.Joiner = &perform.Joiner{
		ServerName: r.Cfg.Matrix.ServerName,
		Cfg:        r.Cfg,
		DB:         r.DB,
		FSAPI:      r.fsAPI,
		Inputer:    r.Inputer,
	}
	r.Peeker = &perform.Peeker{
		ServerName: r.Cfg.Matrix.ServerName,
		Cfg:        r.Cfg,
		DB:         r.DB,
		FSAPI:      r.fsAPI,
		Inputer:    r.Inputer,
	}
	r.InboundPeeker = &perform.InboundPeeker{
		DB:      r.DB,
		Inputer: r.Inputer,
	}
	r.Unpeeker = &perform.Unpeeker{
		ServerName: r.Cfg.Matrix.ServerName,
		Cfg:        r.Cfg,
		DB:         r.DB,
		FSAPI:      r.fsAPI,
		Inputer:    r.Inputer,
	}
	r.Leaver = &perform.Leaver{
		Cfg:     r.Cfg,
		DB:      r.DB,
		FSAPI:   r.fsAPI,
		Inputer: r.Inputer,
	}
	r.Publisher = &perform.Publisher{
		DB: r.DB,
	}
	r.Backfiller = &perform.Backfiller{
		ServerName: r.ServerName,
		DB:         r.DB,
		FSAPI:      r.fsAPI,
		KeyRing:    r.KeyRing,
		// Perspective servers are trusted to not lie about server keys, so we will also
		// prefer these servers when backfilling (assuming they are in the room) rather
		// than trying random servers
		PreferServers: r.PerspectiveServerNames,
	}
	r.Forgetter = &perform.Forgetter{
		DB: r.DB,
	}
}

func (r *RoomserverInternalAPI) SetAppserviceAPI(asAPI asAPI.AppServiceQueryAPI) {
	r.asAPI = asAPI
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

func (r *RoomserverInternalAPI) PerformForget(
	ctx context.Context,
	req *api.PerformForgetRequest,
	resp *api.PerformForgetResponse,
) error {
	return r.Forgetter.PerformForget(ctx, req, resp)
}
