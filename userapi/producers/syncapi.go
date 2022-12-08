package producers

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/userapi/storage"
)

type JetStreamPublisher interface {
	PublishMsg(*nats.Msg, ...nats.PubOpt) (*nats.PubAck, error)
}

// SyncAPI produces messages for the Sync API server to consume.
type SyncAPI struct {
	db                    storage.Database
	producer              JetStreamPublisher
	clientDataTopic       string
	notificationDataTopic string
}

func NewSyncAPI(db storage.Database, js JetStreamPublisher, clientDataTopic string, notificationDataTopic string) *SyncAPI {
	return &SyncAPI{
		db:                    db,
		producer:              js,
		clientDataTopic:       clientDataTopic,
		notificationDataTopic: notificationDataTopic,
	}
}

// SendAccountData sends account data to the Sync API server.
func (p *SyncAPI) SendAccountData(userID string, data eventutil.AccountData) error {
	m := &nats.Msg{
		Subject: p.clientDataTopic,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)

	var err error
	m.Data, err = json.Marshal(data)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   data.RoomID,
		"data_type": data.Type,
	}).Tracef("Producing to topic '%s'", p.clientDataTopic)

	_, err = p.producer.PublishMsg(m)
	return err
}

// GetAndSendNotificationData reads the database and sends data about unread
// notifications to the Sync API server.
func (p *SyncAPI) GetAndSendNotificationData(ctx context.Context, userID, roomID string) error {
	localpart, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return err
	}

	ntotal, nhighlight, err := p.db.GetRoomNotificationCounts(ctx, localpart, domain, roomID)
	if err != nil {
		return err
	}

	return p.sendNotificationData(userID, &eventutil.NotificationData{
		RoomID:                  roomID,
		UnreadHighlightCount:    int(nhighlight),
		UnreadNotificationCount: int(ntotal),
	})
}

// sendNotificationData sends data about unread notifications to the Sync API server.
func (p *SyncAPI) sendNotificationData(userID string, data *eventutil.NotificationData) error {
	m := &nats.Msg{
		Subject: p.notificationDataTopic,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)

	var err error
	m.Data, err = json.Marshal(data)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"user_id": userID,
		"room_id": data.RoomID,
	}).Tracef("Producing to topic '%s'", p.clientDataTopic)

	_, err = p.producer.PublishMsg(m)
	return err
}
