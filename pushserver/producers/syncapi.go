package producers

import (
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal/eventutil"
	log "github.com/sirupsen/logrus"
)

// SyncAPIProducer produces messages for the Sync API server to consume.
type SyncAPIProducer struct {
	Producer        sarama.SyncProducer
	ClientDataTopic string
}

// SendAccountData sends account data to the Sync API server.
func (p *SyncAPIProducer) SendAccountData(userID string, roomID string, dataType string) error {
	var m sarama.ProducerMessage

	data := eventutil.AccountData{
		RoomID: roomID,
		Type:   dataType,
	}
	value, err := json.Marshal(data)
	if err != nil {
		return err
	}

	m.Topic = string(p.ClientDataTopic)
	m.Key = sarama.StringEncoder(userID)
	m.Value = sarama.ByteEncoder(value)
	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   roomID,
		"data_type": dataType,
	}).Infof("Producing to topic '%s'", m.Topic)

	_, _, err = p.Producer.SendMessage(&m)
	return err
}
