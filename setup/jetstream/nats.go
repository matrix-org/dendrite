package jetstream

import (
	"strings"
	"sync"
	"time"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"

	saramajs "github.com/S7evinK/saramajetstream"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	natsclient "github.com/nats-io/nats.go"
)

var natsServer *natsserver.Server
var natsServerMutex sync.Mutex

func Prepare(cfg *config.JetStream) (nats.JetStreamContext, sarama.Consumer, sarama.SyncProducer) {
	// check if we need an in-process NATS Server
	if len(cfg.Addresses) != 0 {
		return setupNATS(cfg, nil)
	}
	natsServerMutex.Lock()
	if natsServer == nil {
		var err error
		natsServer, err = natsserver.NewServer(&natsserver.Options{
			ServerName:       "monolith",
			DontListen:       true,
			JetStream:        true,
			StoreDir:         string(cfg.StoragePath),
			NoSystemAccount:  true,
			AllowNewAccounts: false,
		})
		if err != nil {
			panic(err)
		}
		natsServer.ConfigureLogger()
		go natsServer.Start()
	}
	natsServerMutex.Unlock()
	if !natsServer.ReadyForConnections(time.Second * 10) {
		logrus.Fatalln("NATS did not start in time")
	}
	nc, err := natsclient.Connect("", natsclient.InProcessServer(natsServer))
	if err != nil {
		logrus.Fatalln("Failed to create NATS client")
	}
	return setupNATS(cfg, nc)
}

func setupNATS(cfg *config.JetStream, nc *natsclient.Conn) (nats.JetStreamContext, sarama.Consumer, sarama.SyncProducer) {
	if nc == nil {
		var err error
		nc, err = nats.Connect(strings.Join(cfg.Addresses, ","))
		if err != nil {
			logrus.WithError(err).Panic("Unable to connect to NATS")
			return nil, nil, nil
		}
	}

	s, err := nc.JetStream()
	if err != nil {
		logrus.WithError(err).Panic("Unable to get JetStream context")
		return nil, nil, nil
	}

	for _, stream := range streams { // streams are defined in streams.go
		name := cfg.TopicFor(stream.Name)
		info, err := s.StreamInfo(name)
		if err != nil && err != natsclient.ErrStreamNotFound {
			logrus.WithError(err).Fatal("Unable to get stream info")
		}
		if info == nil {
			stream.Subjects = []string{name}
			// If we're trying to keep everything in memory (e.g. unit tests)
			// then overwrite the storage policy.
			if cfg.InMemory {
				stream.Storage = nats.MemoryStorage
			}

			if _, err = s.AddStream(stream); err != nil {
				logrus.WithError(err).WithField("stream", name).Fatal("Unable to add stream")
			}
		}
	}

	consumer := saramajs.NewJetStreamConsumer(nc, s, "")
	producer := saramajs.NewJetStreamProducer(nc, s, "")
	return s, consumer, producer
}
