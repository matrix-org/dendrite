package consumers

import (
	"context"
	"encoding/json"

	eduapi "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/util"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

type OutputReceiptEventConsumer struct {
	ctx          context.Context
	cfg          *config.UserAPI
	jetstream    nats.JetStreamContext
	durable      string
	db           storage.Database
	pgClient     pushgateway.Client
	receiptTopic string
	syncProducer *producers.SyncAPI
}

// NewOutputReceiptEventConsumer creates a new OutputEDUConsumer. Call Start() to begin consuming from EDU servers.
func NewOutputReceiptEventConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.Database,
	pgClient pushgateway.Client,
	syncProducer *producers.SyncAPI,
) *OutputReceiptEventConsumer {
	return &OutputReceiptEventConsumer{
		ctx:          process.Context(),
		cfg:          cfg,
		jetstream:    js,
		db:           store,
		durable:      cfg.Matrix.JetStream.Durable("UserAPIEDUServerConsumer"),
		receiptTopic: cfg.Matrix.JetStream.TopicFor(jetstream.OutputReceiptEvent),
		pgClient:     pgClient,
		syncProducer: syncProducer,
	}
}

func (s *OutputReceiptEventConsumer) Start() error {
	if err := jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.receiptTopic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	return nil
}

func (s *OutputReceiptEventConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	var event eduapi.OutputReceiptEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		log.WithError(err).Errorf("pushserver EDU consumer: message parse failure")
		return true
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', event.UserID)
	if err != nil {
		return true
	}
	if domain != s.cfg.Matrix.ServerName {
		return true
	}

	log.WithFields(log.Fields{
		"localpart":  localpart,
		"room_id":    event.RoomID,
		"event_id":   event.EventID,
		"event_type": event.Type,
	}).Tracef("Received message from EDU server: %#v", event)

	// TODO: we cannot know if this EventID caused a notification, so
	// we should first resolve it and find the closest earlier
	// notification.
	updated, err := s.db.SetNotificationsRead(ctx, localpart, event.RoomID, event.EventID, true)
	if err != nil {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
			"event_id":  event.EventID,
		}).WithError(err).Error("pushserver EDU consumer")
		return false
	}

	if updated {
		if err := s.syncProducer.GetAndSendNotificationData(ctx, event.UserID, event.RoomID); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  event.EventID,
			}).WithError(err).Error("pushserver EDU consumer: GetAndSendNotificationData failed")
			return false
		}

		if err := util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  event.EventID,
			}).WithError(err).Error("pushserver EDU consumer: NotifyUserCounts failed")
			return false
		}

	}

	return true
}
