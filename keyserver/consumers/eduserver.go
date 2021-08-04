package consumers

import (
	"fmt"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"

	"github.com/Shopify/sarama"
)

type OutputSigningKeyUpdateConsumer struct {
	eduServerConsumer *internal.ContinualConsumer
	keyDB             storage.Database
	keyAPI            api.KeyInternalAPI
	serverName        string
}

func NewOutputSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	keyDB storage.Database,
	keyAPI api.KeyInternalAPI,
) *OutputSigningKeyUpdateConsumer {
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "keyserver/eduserver",
		Topic:          cfg.Global.Kafka.TopicFor(config.TopicOutputSigningKeyUpdate),
		Consumer:       kafkaConsumer,
		PartitionStore: keyDB,
	}
	s := &OutputSigningKeyUpdateConsumer{
		eduServerConsumer: &consumer,
		keyDB:             keyDB,
		keyAPI:            keyAPI,
		serverName:        string(cfg.Global.ServerName),
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

func (s *OutputSigningKeyUpdateConsumer) Start() error {
	return s.eduServerConsumer.Start()
}

func (s *OutputSigningKeyUpdateConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	/*
		var output eduapi.OutputSigningKeyUpdate
		if err := json.Unmarshal(msg.Value, &output); err != nil {
			log.WithError(err).Errorf("eduserver output log: message parse failure")
			return nil
		}
		return nil
	*/
	return fmt.Errorf("TODO")
}
