package consumers

import (
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	log "github.com/sirupsen/logrus"
)

type OutputRoomEventConsumer struct {
	cfg        *config.PushServer
	psAPI      api.PushserverInternalAPI
	rsConsumer *internal.ContinualConsumer
	db         storage.Database
}

func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.PushServer,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
	psAPI api.PushserverInternalAPI,
) *OutputRoomEventConsumer {
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "pushserver/roomserver",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputRoomEvent)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEventConsumer{
		cfg:        cfg,
		rsConsumer: &consumer,
		db:         store,
		psAPI:      psAPI,
	}
	consumer.ProcessMessage = s.onMessage
	return s
}

func (s *OutputRoomEventConsumer) Start() error {
	return s.rsConsumer.Start()
}

func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output rsapi.OutputEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	switch output.Type {
	case rsapi.OutputTypeNewRoomEvent:
		ev := output.NewRoomEvent.Event
		if err := s.processMessage(*output.NewRoomEvent); err != nil {
			// panic rather than continue with an inconsistent database
			log.WithFields(log.Fields{
				"event_id":   ev.EventID(),
				"event":      string(ev.JSON()),
				log.ErrorKey: err,
			}).Panicf("roomserver output log: write room event failure")
			return err
		}

	default:
		// Ignore old events, peeks, so on.
	}

	return nil
}

func (s *OutputRoomEventConsumer) processMessage(ore rsapi.OutputNewRoomEvent) error {
	// TODO: New events from the roomserver will be passed here.

	return nil
}
