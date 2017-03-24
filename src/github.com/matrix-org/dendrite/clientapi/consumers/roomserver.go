package consumers

import (
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/common"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// RoomserverConsumer consumes events from the room server
type RoomserverConsumer struct {
	Consumer common.ContinualConsumer
}

// NewRoomserverConsumer creates a new roomserver consumer
func NewRoomserverConsumer(cfg *config.Sync, store common.PartitionStorer) (*RoomserverConsumer, error) {
	kafkaConsumer, err := sarama.NewConsumer(cfg.KafkaConsumerURIs, nil)
	if err != nil {
		return nil, err
	}

	return &RoomserverConsumer{
		Consumer: common.ContinualConsumer{
			Topic:          cfg.RoomserverOutputTopic,
			Consumer:       kafkaConsumer,
			PartitionStore: store,
		},
	}, nil

}
