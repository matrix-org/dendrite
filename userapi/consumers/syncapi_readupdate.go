package consumers

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/types"
	uapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/producers"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/util"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

type OutputReadUpdateConsumer struct {
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

func NewOutputReadUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.UserAPI,
	js nats.JetStreamContext,
	store storage.Database,
	pgClient pushgateway.Client,
	userAPI uapi.UserInternalAPI,
	syncProducer *producers.SyncAPI,
) *OutputReadUpdateConsumer {
	return &OutputReadUpdateConsumer{
		ctx:          process.Context(),
		cfg:          cfg,
		jetstream:    js,
		db:           store,
		ServerName:   cfg.Matrix.ServerName,
		durable:      cfg.Matrix.JetStream.Durable("UserAPISyncAPIReadUpdateConsumer"),
		topic:        cfg.Matrix.JetStream.TopicFor(jetstream.OutputReadUpdate),
		pgClient:     pgClient,
		userAPI:      userAPI,
		syncProducer: syncProducer,
	}
}

func (s *OutputReadUpdateConsumer) Start() error {
	if err := jetstream.JetStreamConsumer(
		s.ctx, s.jetstream, s.topic, s.durable, s.onMessage,
		nats.DeliverAll(), nats.ManualAck(),
	); err != nil {
		return err
	}
	return nil
}

func (s *OutputReadUpdateConsumer) onMessage(ctx context.Context, msg *nats.Msg) bool {
	var read types.ReadUpdate
	if err := json.Unmarshal(msg.Data, &read); err != nil {
		log.WithError(err).Error("userapi clientapi consumer: message parse failure")
		return true
	}
	if read.FullyRead == 0 && read.Read == 0 {
		return true
	}

	userID := string(msg.Header.Get(jetstream.UserID))
	roomID := string(msg.Header.Get(jetstream.RoomID))

	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		log.WithError(err).Error("userapi clientapi consumer: SplitID failure")
		return true
	}
	if domain != s.ServerName {
		log.Error("userapi clientapi consumer: not a local user")
		return true
	}

	log := log.WithFields(log.Fields{
		"room_id": roomID,
		"user_id": userID,
	})
	log.Tracef("Received read update from sync API: %#v", read)

	if read.Read > 0 {
		updated, err := s.db.SetNotificationsRead(ctx, localpart, roomID, int64(read.Read), true)
		if err != nil {
			log.WithError(err).Error("userapi EDU consumer")
			return false
		}

		if updated {
			if err = s.syncProducer.GetAndSendNotificationData(ctx, userID, roomID); err != nil {
				log.WithError(err).Error("userapi EDU consumer: GetAndSendNotificationData failed")
				return false
			}
			if err = util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
				log.WithError(err).Error("userapi EDU consumer: NotifyUserCounts failed")
				return false
			}
		}
	}

	if read.FullyRead > 0 {
		deleted, err := s.db.DeleteNotificationsUpTo(ctx, localpart, roomID, int64(read.FullyRead))
		if err != nil {
			log.WithError(err).Errorf("userapi clientapi consumer: DeleteNotificationsUpTo failed")
			return false
		}

		if deleted {
			if err := util.NotifyUserCountsAsync(ctx, s.pgClient, localpart, s.db); err != nil {
				log.WithError(err).Error("userapi clientapi consumer: NotifyUserCounts failed")
				return false
			}

			if err := s.syncProducer.GetAndSendNotificationData(ctx, userID, read.RoomID); err != nil {
				log.WithError(err).Errorf("userapi clientapi consumer: GetAndSendNotificationData failed")
				return false
			}
		}
	}

	return true
}
