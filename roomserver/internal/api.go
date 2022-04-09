package internal

import (
	"context"

	"github.com/getsentry/sentry-go"
	asAPI "github.com/matrix-org/dendrite/appservice/api"
	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/internal/perform"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
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
	*perform.Upgrader
	ProcessContext         *process.ProcessContext
	DB                     storage.Database
	Cfg                    *config.RoomServer
	Cache                  caching.RoomServerCaches
	ServerName             gomatrixserverlib.ServerName
	KeyRing                gomatrixserverlib.JSONVerifier
	ServerACLs             *acls.ServerACLs
	fsAPI                  fsAPI.FederationInternalAPI
	asAPI                  asAPI.AppServiceQueryAPI
	NATSClient             *nats.Conn
	JetStream              nats.JetStreamContext
	Durable                string
	InputRoomEventTopic    string // JetStream topic for new input room events
	OutputRoomEventTopic   string // JetStream topic for new output room events
	PerspectiveServerNames []gomatrixserverlib.ServerName
}

func NewRoomserverAPI(
	processCtx *process.ProcessContext, cfg *config.RoomServer, roomserverDB storage.Database,
	consumer nats.JetStreamContext, nc *nats.Conn,
	inputRoomEventTopic, outputRoomEventTopic string,
	caches caching.RoomServerCaches, perspectiveServerNames []gomatrixserverlib.ServerName,
) *RoomserverInternalAPI {
	serverACLs := acls.NewServerACLs(roomserverDB)
	a := &RoomserverInternalAPI{
		ProcessContext:         processCtx,
		DB:                     roomserverDB,
		Cfg:                    cfg,
		Cache:                  caches,
		ServerName:             cfg.Matrix.ServerName,
		PerspectiveServerNames: perspectiveServerNames,
		InputRoomEventTopic:    inputRoomEventTopic,
		OutputRoomEventTopic:   outputRoomEventTopic,
		JetStream:              consumer,
		NATSClient:             nc,
		Durable:                cfg.Matrix.JetStream.Durable("RoomserverInputConsumer"),
		ServerACLs:             serverACLs,
		Queryer: &query.Queryer{
			DB:         roomserverDB,
			Cache:      caches,
			ServerName: cfg.Matrix.ServerName,
			ServerACLs: serverACLs,
		},
		// perform-er structs get initialised when we have a federation sender to use
	}
	return a
}

// SetFederationInputAPI passes in a federation input API reference so that we can
// avoid the chicken-and-egg problem of both the roomserver input API and the
// federation input API being interdependent.
func (r *RoomserverInternalAPI) SetFederationAPI(fsAPI fsAPI.FederationInternalAPI, keyRing *gomatrixserverlib.KeyRing) {
	r.fsAPI = fsAPI
	r.KeyRing = keyRing

	r.Inputer = &input.Inputer{
		Cfg:                  r.Cfg,
		ProcessContext:       r.ProcessContext,
		DB:                   r.DB,
		InputRoomEventTopic:  r.InputRoomEventTopic,
		OutputRoomEventTopic: r.OutputRoomEventTopic,
		JetStream:            r.JetStream,
		NATSClient:           r.NATSClient,
		Durable:              nats.Durable(r.Durable),
		ServerName:           r.Cfg.Matrix.ServerName,
		FSAPI:                fsAPI,
		KeyRing:              keyRing,
		ACLs:                 r.ServerACLs,
		Queryer:              r.Queryer,
	}
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
		RSAPI:      r,
		Inputer:    r.Inputer,
		Queryer:    r.Queryer,
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
	r.Upgrader = &perform.Upgrader{
		Cfg:    r.Cfg,
		URSAPI: r,
	}

	if err := r.Inputer.Start(); err != nil {
		logrus.WithError(err).Panic("failed to start roomserver input API")
	}
}

func (r *RoomserverInternalAPI) SetUserAPI(userAPI userapi.UserInternalAPI) {
	r.Leaver.UserAPI = userAPI
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
		sentry.CaptureException(err)
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
		sentry.CaptureException(err)
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
