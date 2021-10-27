package consumers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Shopify/sarama"
	eduapi "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/pushserver/producers"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/pushserver/util"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type OutputReceiptEventConsumer struct {
	cfg          *config.PushServer
	rsConsumer   *internal.ContinualConsumer
	db           storage.Database
	pgClient     pushgateway.Client
	syncProducer *producers.SyncAPI
}

func NewOutputReceiptEventConsumer(
	process *process.ProcessContext,
	cfg *config.PushServer,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
	pgClient pushgateway.Client,
	syncProducer *producers.SyncAPI,
) *OutputReceiptEventConsumer {
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "pushserver/eduserver",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputReceiptEvent)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputReceiptEventConsumer{
		cfg:          cfg,
		rsConsumer:   &consumer,
		db:           store,
		pgClient:     pgClient,
		syncProducer: syncProducer,
	}
	consumer.ProcessMessage = s.onMessage
	return s
}

func (s *OutputReceiptEventConsumer) Start() error {
	return s.rsConsumer.Start()
}

func (s *OutputReceiptEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	ctx := context.Background()

	var event eduapi.OutputReceiptEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.WithError(err).Errorf("pushserver EDU consumer: message parse failure")
		return nil
	}

	localpart, domain, err := gomatrixserverlib.SplitID('@', event.UserID)
	if err != nil {
		return err
	}

	if domain != s.cfg.Matrix.ServerName {
		return fmt.Errorf("pushserver EDU consumer: not a local user: %v", event.UserID)
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
		return nil
	}

	if updated {
		if err := util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  event.EventID,
			}).WithError(err).Error("pushserver EDU consumer: NotifyUserCounts failed")
			return nil
		}

		if err := s.syncProducer.GetAndSendNotificationData(ctx, event.UserID, event.RoomID); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  event.EventID,
			}).WithError(err).Error("pushserver EDU consumer: GetAndSendNotificationData failed")
			return nil
		}
	}

	return nil
}
