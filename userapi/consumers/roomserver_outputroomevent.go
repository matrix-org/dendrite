package consumers

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi/storage"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	ctx        context.Context
	jetstream  nats.JetStreamContext
	durable    string
	topic      string
	userDB     storage.Database
	serverName gomatrixserverlib.ServerName
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call
// Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	base *base.BaseDendrite,
	js nats.JetStreamContext,
	userDB storage.Database,
) *OutputRoomEventConsumer {
	return &OutputRoomEventConsumer{
		ctx:        base.Context(),
		jetstream:  js,
		durable:    base.Cfg.Global.JetStream.Durable("UserAPIRoomserverConsumer"),
		topic:      base.Cfg.Global.JetStream.Prefixed(jetstream.OutputRoomEvent),
		userDB:     userDB,
		serverName: base.Cfg.Global.ServerName,
	}
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	)
}

// onMessage is called when the userapi component receives a new event from
// the room server output log.
func (s *OutputRoomEventConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	// Parse out the event JSON
	var output api.OutputEvent
	if err := json.Unmarshal(msg.Data, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return true
	}

	log.WithFields(log.Fields{
		"type": output.Type,
	}).Debug("Got a message in OutputRoomEventConsumer")

	if output.Type == api.OutputTypeNewRoomEvent && output.NewRoomEvent != nil {
		ev := output.NewRoomEvent.Event
		// Only handle membership events
		if ev.Type() != gomatrixserverlib.MRoomMember || ev.StateKey() == nil {
			return true
		}
		localPart, domain, err := gomatrixserverlib.SplitID('@', *ev.StateKey())
		if err != nil {
			return true
		}
		// Profiles from ourselves are updated by API calls, don't delete them.
		if domain == s.serverName {
			return true
		}
		log.WithField("user_id", *ev.StateKey()).Debug("Deleting remote user profile")
		if err := s.userDB.DeleteProfile(ctx, localPart, domain); err != nil {
			// non-fatal error, log and continue
			log.WithError(err).WithField("user_id", *ev.StateKey()).Warn("failed to delete user profile")
		}
	}

	return true
}
