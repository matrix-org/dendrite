package sync

import (
	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/common"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// Server contains all the logic for running a sync server
type Server struct {
	roomServerConsumer *common.ContinualConsumer
}

// NewServer creates a new sync server. Call Start() to begin consuming from room servers.
func NewServer(cfg *config.Sync, store common.PartitionStorer) (*Server, error) {
	kafkaConsumer, err := sarama.NewConsumer(cfg.KafkaConsumerURIs, nil)
	if err != nil {
		return nil, err
	}

	consumer := common.ContinualConsumer{
		Topic:          cfg.RoomserverOutputTopic,
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &Server{
		roomServerConsumer: &consumer,
	}
	consumer.ProcessMessage = s.onMessage

	return s, nil
}

// Start consuming from room servers
func (s *Server) Start() error {
	return s.roomServerConsumer.Start()
}

func (s *Server) onMessage(msg *sarama.ConsumerMessage) error {
	log.WithField("key", string(msg.Key)).WithField("val", string(msg.Value)).Info("Recv")
	return nil
}
