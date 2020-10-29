package kafka

import (
	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/naffka"
	naffkaStorage "github.com/matrix-org/naffka/storage"
	"github.com/sirupsen/logrus"
)

func SetupConsumerProducer(cfg *config.Kafka) (sarama.Consumer, sarama.SyncProducer) {
	if cfg.UseNaffka {
		return setupNaffka(cfg)
	}
	return setupKafka(cfg)
}

// setupKafka creates kafka consumer/producer pair from the config.
func setupKafka(cfg *config.Kafka) (sarama.Consumer, sarama.SyncProducer) {
	sCfg := sarama.NewConfig()
	sCfg.Producer.MaxMessageBytes = *cfg.MaxMessageBytes
	sCfg.Producer.Return.Successes = true
	sCfg.Consumer.Fetch.Default = int32(*cfg.MaxMessageBytes)

	consumer, err := sarama.NewConsumer(cfg.Addresses, sCfg)
	if err != nil {
		logrus.WithError(err).Panic("failed to start kafka consumer")
	}

	producer, err := sarama.NewSyncProducer(cfg.Addresses, sCfg)
	if err != nil {
		logrus.WithError(err).Panic("failed to setup kafka producers")
	}

	return consumer, producer
}

// In monolith mode with Naffka, we don't have the same constraints about
// consuming the same topic from more than one place like we do with Kafka.
// Therefore, we will only open one Naffka connection in case Naffka is
// running on SQLite.
var naffkaInstance *naffka.Naffka

// setupNaffka creates kafka consumer/producer pair from the config.
func setupNaffka(cfg *config.Kafka) (sarama.Consumer, sarama.SyncProducer) {
	if naffkaInstance != nil {
		return naffkaInstance, naffkaInstance
	}
	naffkaDB, err := naffkaStorage.NewDatabase(string(cfg.Database.ConnectionString))
	if err != nil {
		logrus.WithError(err).Panic("Failed to setup naffka database")
	}
	naffkaInstance, err = naffka.New(naffkaDB)
	if err != nil {
		logrus.WithError(err).Panic("Failed to setup naffka")
	}
	return naffkaInstance, naffkaInstance
}
