package kafka

import (
	"time"

	js "github.com/S7evinK/saramajetstream"
	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/naffka"
	naffkaStorage "github.com/matrix-org/naffka/storage"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

func SetupConsumerProducer(cfg *config.Kafka) (sarama.Consumer, sarama.SyncProducer) {
	if cfg.UseNaffka {
		return setupNaffka(cfg)
	}
	if cfg.UseNATS {
		return setupNATS(cfg)
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

func setupNATS(cfg *config.Kafka) (sarama.Consumer, sarama.SyncProducer) {
	logrus.WithField("servers", cfg.Addresses).Debug("connecting to nats")
	nc, err := nats.Connect(cfg.Addresses[0])
	if err != nil {
		logrus.WithError(err).Panic("failed to connect to nats")
		return nil, nil
	}

	s, err := nc.JetStream()
	if err != nil {
		logrus.WithError(err).Panic("unable to get jetstream context")
		return nil, nil
	}

	// create a stream for every topic
	for _, topic := range config.KafkaTopics {
		sn := cfg.TopicFor(topic)
		stream, err := s.StreamInfo(sn)
		if err != nil {
			logrus.WithError(err).Warn("unable to get stream info")
		}

		if stream == nil {
			maxLifeTime := time.Second * 0

			// Typing events can be removed from the stream, as they are only relevant for a short time
			if topic == config.TopicOutputTypingEvent {
				maxLifeTime = time.Second * 30
			}
			_, _ = s.AddStream(&nats.StreamConfig{
				Name:       sn,
				Subjects:   []string{topic},
				MaxBytes:   int64(*cfg.MaxMessageBytes),
				MaxMsgSize: int32(*cfg.MaxMessageBytes),
				MaxAge:     maxLifeTime,
				Duplicates: maxLifeTime / 2,
			})
		}
	}

	consumer := js.NewJetStreamConsumer(s, cfg.TopicPrefix)
	producer := js.NewJetStreamProducer(s, cfg.TopicPrefix)
	return consumer, producer
}
