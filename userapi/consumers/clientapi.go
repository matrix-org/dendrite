package consumers

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/util"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

type OutputClientDataConsumer struct {
	ctx          context.Context
	cfg          *config.UserAPI
	jetstream    nats.JetStreamContext
	durable      string
	db           storage.Database
	pgClient     pushgateway.Client
	ServerName   gomatrixserverlib.ServerName
	topic        string
	userAPI      uapi.UserInternalAPI
	syncProducer *producers.SyncAPI
}

func NewOutputClientDataConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.Database,
	pgClient pushgateway.Client,
	userAPI uapi.UserInternalAPI,
	syncProducer *producers.SyncAPI,
) *OutputClientDataConsumer {
	return &OutputClientDataConsumer{
		ctx:          process.Context(),
		cfg:          cfg,
		jetstream:    js,
		db:           store,
		ServerName:   cfg.Matrix.ServerName,
		durable:      cfg.Matrix.JetStream.Durable("UserAPIClientAPIConsumer"),
		topic:        cfg.Matrix.JetStream.TopicFor(jetstream.OutputClientData),
		pgClient:     pgClient,
		userAPI:      userAPI,
		syncProducer: syncProducer,
	}
}

func (s *OutputClientDataConsumer) Start() error {
	if err := jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	return nil
}

func (s *OutputClientDataConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	var event eventutil.AccountData
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		log.WithError(err).Error("pushserver clientapi consumer: message parse failure")
		return true
	}

	if event.Type != mFullyRead {
		return true
	}

	userID := string(msg.Header.Get("user_id"))
	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithFields(log.Fields{
			"user_id":    userID,
			"room_id":    event.RoomID,
			"event_type": event.Type,
		}).WithError(err).Error("pushserver clientapi consumer: SplitID failure")
		return true
	}

	if domain != s.ServerName {
		log.WithFields(log.Fields{
			"user_id":    userID,
			"room_id":    event.RoomID,
			"event_type": event.Type,
		}).Error("pushserver clientapi consumer: not a local user")
		return true
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
	if err = s.userAPI.QueryAccountData(ctx, &userReq, &userRes); err != nil {
		log.WithFields(log.Fields{
			"localpart":  localpart,
			"room_id":    event.RoomID,
			"event_type": event.Type,
		}).WithError(err).Error("pushserver clientapi consumer: failed to query account data")
		return false
	}
	ad, ok := userRes.RoomAccountData[event.RoomID]
	if !ok {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
		}).Errorf("pushserver clientapi consumer: room not found in account data response: %#v", userRes.RoomAccountData)
		return true
	}
	bs, ok := ad[mFullyRead]
	if !ok {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
		}).Errorf("pushserver clientapi consumer: m.fully_read not found in account data: %#v", ad)
		return true
	}
	var data fullyReadAccountData
	if err = json.Unmarshal([]byte(bs), &data); err != nil {
		log.WithFields(log.Fields{
			"localpart": localpart,
			"room_id":   event.RoomID,
		}).WithError(err).Error("pushserver clientapi consumer: json.Unmarshal of m.fully_read failed")
		return true
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
		return false
	}

	if deleted {
		if err := util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  data.EventID,
			}).WithError(err).Error("pushserver clientapi consumer: NotifyUserCounts failed")
			return false
		}

		if err := s.syncProducer.GetAndSendNotificationData(ctx, userID, event.RoomID); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
				"room_id":   event.RoomID,
				"event_id":  data.EventID,
			}).WithError(err).Errorf("pushserver clientapi consumer: GetAndSendNotificationData failed")
			return false
		}
	}

	return true
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
