package consumers

import (
	"context"
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/pushserver/producers"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/pushserver/util"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type OutputClientDataConsumer struct {
	cfg          *config.PushServer
	rsConsumer   *internal.ContinualConsumer
	db           storage.Database
	pgClient     pushgateway.Client
	userAPI      uapi.UserInternalAPI
	syncProducer *producers.SyncAPI
}

func NewOutputClientDataConsumer(
	process *process.ProcessContext,
	cfg *config.PushServer,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
	pgClient pushgateway.Client,
	userAPI uapi.UserInternalAPI,
	syncProducer *producers.SyncAPI,
) *OutputClientDataConsumer {
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "pushserver/clientapi",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputClientData)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputClientDataConsumer{
		cfg:          cfg,
		rsConsumer:   &consumer,
		db:           store,
		pgClient:     pgClient,
		userAPI:      userAPI,
		syncProducer: syncProducer,
	}
	consumer.ProcessMessage = s.onMessage
	return s
}

func (s *OutputClientDataConsumer) Start() error {
	return s.rsConsumer.Start()
}

func (s *OutputClientDataConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	ctx := context.Background()

	var event eventutil.AccountData
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		log.WithError(err).Error("pushserver clientapi consumer: message parse failure")
		return nil
	}

	if event.Type != mFullyRead {
		return nil
	}

	userID := string(msg.Key)
	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithFields(log.Fields{
			"user_id":    userID,
			"room_id":    event.RoomID,
			"event_type": event.Type,
		}).WithError(err).Error("pushserver clientapi consumer: SplitID failure")
		return nil
	}

	if domain != s.cfg.Matrix.ServerName {
		log.WithFields(log.Fields{
			"user_id":    userID,
			"room_id":    event.RoomID,
			"event_type": event.Type,
		}).Error("pushserver clientapi consumer: not a local user")
		return nil
	}

	log.WithFields(log.Fields{
		"localpart":  localpart,
		"room_id":    event.RoomID,
		"event_type": event.Type,
	}).Tracef("Received message from clientapi: %#v", event)

	userReq := uapi.QueryAccountDataRequest{
		UserID:   userID,
		RoomID:   event.RoomID,
		DataType: mFullyRead,
	}
	var userRes uapi.QueryAccountDataResponse
	if err := s.userAPI.QueryAccountData(ctx, &userReq, &userRes); err != nil {
		log.WithFields(log.Fields{
			"localpart":  localpart,
			"room_id":    event.RoomID,
			"event_type": event.Type,
		}).WithError(err).Error("pushserver clientapi consumer: failed to query account data")
		return nil
	}
	ad, ok := userRes.RoomAccountData[event.RoomID]
	if !ok {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
		}).Errorf("pushserver clientapi consumer: room not found in account data response: %#v", userRes.RoomAccountData)
		return nil
	}
	bs, ok := ad[mFullyRead]
	if !ok {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
		}).Errorf("pushserver clientapi consumer: m.fully_read not found in account data: %#v", ad)
		return nil
	}
	var data fullyReadAccountData
	if err := json.Unmarshal([]byte(bs), &data); err != nil {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
		}).WithError(err).Error("pushserver clientapi consumer: json.Unmarshal of m.fully_read failed")
		return nil
	}

	// TODO: we cannot know if this EventID caused a notification, so
	// we should first resolve it and find the closest earlier
	// notification.
	deleted, err := s.db.DeleteNotificationsUpTo(ctx, localpart, event.RoomID, data.EventID)
	if err != nil {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
			"event_id":  data.EventID,
		}).WithError(err).Errorf("pushserver clientapi consumer: DeleteNotificationsUpTo failed")
		return nil
	}

	if deleted {
		if err := util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  data.EventID,
			}).WithError(err).Error("pushserver clientapi consumer: NotifyUserCounts failed")
			return nil
		}

		if err := s.syncProducer.GetAndSendNotificationData(ctx, userID, event.RoomID); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  data.EventID,
			}).WithError(err).Errorf("pushserver clientapi consumer: GetAndSendNotificationData failed")
			return nil
		}
	}

	return nil
}

// mFullyRead is the account data type for the marker for the event up
// to which the user has read.
const mFullyRead = "m.fully_read"

// A fullyReadAccountData is what the m.fully_read account data value
// contains.
//
// TODO: this is duplicated with
// clientapi/routing/account_data.go. Should probably move to
// eventutil.
type fullyReadAccountData struct {
	EventID string `json:"event_id"`
}
