package kafka

import (
	"strings"
	"sync"
	"time"

	js "github.com/S7evinK/saramajetstream"
	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/naffka"
	naffkaStorage "github.com/matrix-org/naffka/storage"
	"github.com/nats-io/nats-server/v2/server"
	natsserver "github.com/nats-io/nats-server/v2/server"
	natsclient "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

var natsServer *natsserver.Server
var natsServerMutex sync.Mutex

func SetupConsumerProducer(cfg *config.Kafka) (sarama.Consumer, sarama.SyncProducer) {
	/*
	if cfg.UseNaffka {
		return setupNaffka(cfg)
	}
	*/
	if true || cfg.UseNATS {
		if true {
			natsServerMutex.Lock()
			s := natsServer
			if s == nil {
				logrus.Infof("Starting NATS")
				var err error
				natsServer, err = natsserver.NewServer(&server.Options{
					ServerName: "monolith",
					DontListen: true,
					JetStream: true,
					StoreDir: string(cfg.Matrix.ServerName),
					LogFile: "nats.log",
					Debug: true,
				})
				if err != nil {
					panic(err)
				}
				natsServer.ConfigureLogger()
				go natsServer.Start()
				s = natsServer
			}
			natsServerMutex.Unlock()
			if !natsServer.ReadyForConnections(time.Second * 10) {
				logrus.Fatalln("NATS did not start in time")
			}
			conn, err := s.InProcessConn()
			if err != nil {
				logrus.Fatalln("Failed to get a NATS in-process conn")
			}
			nc, err := natsclient.Connect("", natsclient.InProcessConn(conn))
			if err != nil {
				logrus.Fatalln("Failed to create NATS client")
			}
			return setupNATS(cfg, nc)
		} else {
			return setupNATS(cfg, nil)
		}
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

func setupNATS(cfg *config.Kafka, nc *natsclient.Conn) (sarama.Consumer, sarama.SyncProducer) {
	logrus.WithField("servers", cfg.Addresses).Debug("connecting to nats")

	if nc == nil {
		var err error
		nc, err = nats.Connect(strings.Join(cfg.Addresses, ","))
		if err != nil {
			logrus.WithError(err).Panic("unable to connect to NATS")
			return nil, nil
		}
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
				maxLifeTime = time.Second * 60
			}
			_, err = s.AddStream(&nats.StreamConfig{
				Name:       sn,
				Subjects:   []string{topic},
				MaxBytes:   int64(*cfg.MaxMessageBytes),
				MaxMsgSize: int32(*cfg.MaxMessageBytes),
				MaxAge:     maxLifeTime,
				Duplicates: maxLifeTime / 2,
			})
			if err != nil {
				logrus.WithError(err).WithField("stream", sn).Fatal("unable to add nats stream")
			}
		}
	}

	consumer := js.NewJetStreamConsumer(nc, s, cfg.TopicPrefix)
	producer := js.NewJetStreamProducer(nc, s, cfg.TopicPrefix)
	return consumer, producer
}
