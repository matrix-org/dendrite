package producers

import (
	"context"
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// SyncAPI produces messages for the Sync API server to consume.
type SyncAPI struct {
	db                    storage.Database
	producer              MessageSender
	clientDataTopic       string
	notificationDataTopic string
}

func NewSyncAPI(db storage.Database, producer MessageSender, clientDataTopic string, notificationDataTopic string) *SyncAPI {
	return &SyncAPI{
		db:                    db,
		producer:              producer,
		clientDataTopic:       clientDataTopic,
		notificationDataTopic: notificationDataTopic,
	}
}

// SendAccountData sends account data to the Sync API server.
func (p *SyncAPI) SendAccountData(userID string, roomID string, dataType string) error {
	var m sarama.ProducerMessage

	data := eventutil.AccountData{
		RoomID: roomID,
		Type:   dataType,
	}
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}

	m.Topic = string(p.clientDataTopic)
	m.Key = sarama.StringEncoder(userID)
	m.Value = sarama.ByteEncoder(value)
	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   roomID,
		"data_type": dataType,
	}).Infof("Producing to topic %q", m.Topic)

	_, _, err = p.producer.SendMessage(&m)
	return err
}

// GetAndSendNotificationData reads the database and sends data about unread
// notifications to the Sync API server.
func (p *SyncAPI) GetAndSendNotificationData(ctx context.Context, userID, roomID string) error {
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return err
	}

	ntotal, nhighlight, err := p.db.GetRoomNotificationCounts(ctx, localpart, roomID)
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
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}

	var m sarama.ProducerMessage
	m.Topic = string(p.notificationDataTopic)
	m.Key = sarama.StringEncoder(userID)
	m.Value = sarama.ByteEncoder(value)
	log.WithFields(log.Fields{
		"user_id": userID,
		"room_id": data.RoomID,
	}).Infof("Producing to topic %q", m.Topic)

	_, _, err = p.producer.SendMessage(&m)
	return err
}

type MessageSender interface {
	SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error)
}
