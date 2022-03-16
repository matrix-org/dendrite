package jetstream

import (
	"strings"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/sirupsen/logrus"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsclient "github.com/nats-io/nats.go"
)

var natsServer *natsserver.Server
var natsServerMutex sync.Mutex

func Prepare(cfg *config.JetStream) (natsclient.JetStreamContext, *natsclient.Conn) {
	// check if we need an in-process NATS Server
	if len(cfg.Addresses) != 0 {
		return setupNATS(cfg, nil)
	}
	natsServerMutex.Lock()
	if natsServer == nil {
		var err error
		natsServer, err = natsserver.NewServer(&natsserver.Options{
			ServerName:      "monolith",
			DontListen:      true,
			JetStream:       true,
			StoreDir:        string(cfg.StoragePath),
			NoSystemAccount: true,
			MaxPayload:      16 * 1024 * 1024,
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

func setupNATS(cfg *config.JetStream, nc *natsclient.Conn) (natsclient.JetStreamContext, *natsclient.Conn) {
	if nc == nil {
		var err error
		nc, err = natsclient.Connect(strings.Join(cfg.Addresses, ","))
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
				stream.Storage = natsclient.MemoryStorage
			}

			// Namespace the streams without modifying the original streams
			// array, otherwise we end up with namespaces on namespaces.
			namespaced := *stream
			namespaced.Name = name
			if _, err = s.AddStream(&namespaced); err != nil {
				logrus.WithError(err).WithField("stream", name).Fatal("Unable to add stream")
			}
		}
	}

	return s, nc
}
