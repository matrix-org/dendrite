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

func SetupConsumerProducer(cfg *config.JetStream) (sarama.Consumer, sarama.SyncProducer) {
	natsServerMutex.Lock()
	s := natsServer
	if s == nil {
		var err error
		natsServer, err = natsserver.NewServer(&natsserver.Options{
			ServerName: "monolith",
			DontListen: true,
			JetStream:  true,
			StoreDir:   string(cfg.Matrix.ServerName),
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
}

func setupNATS(cfg *config.JetStream, nc *natsclient.Conn) (sarama.Consumer, sarama.SyncProducer) {
	if nc == nil {
		var err error
		nc, err = nats.Connect(strings.Join(cfg.Addresses, ","))
		if err != nil {
			logrus.WithError(err).Panic("Unable to connect to NATS")
			return nil, nil
		}
	}

	s, err := nc.JetStream()
	if err != nil {
		logrus.WithError(err).Panic("Unable to get JetStream context")
		return nil, nil
	}

	for _, stream := range streams {
		info, err := s.StreamInfo(stream.Name)
		if err != nil && err != natsclient.ErrStreamNotFound {
			logrus.WithError(err).Fatal("Unable to get stream info")
		}
		if info == nil {
			stream.Name = cfg.TopicFor(stream.Name)
			stream.Subjects = []string{stream.Name}

			if _, err = s.AddStream(stream); err != nil {
				logrus.WithError(err).WithField("stream", stream.Name).Fatal("Unable to add stream")
			}
		}
	}

	consumer := saramajs.NewJetStreamConsumer(nc, s, "")
	producer := saramajs.NewJetStreamProducer(nc, s, "")
	return consumer, producer
}
