package jetstream

import (
	"crypto/tls"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	natsclient "github.com/nats-io/nats.go"
)

type NATSInstance struct {
	*natsserver.Server
	nc *natsclient.Conn
	js natsclient.JetStreamContext
}

var natsLock sync.Mutex

func DeleteAllStreams(js natsclient.JetStreamContext, cfg *config.JetStream) {
	for _, stream := range streams { // streams are defined in streams.go
		name := cfg.Prefixed(stream.Name)
		_ = js.DeleteStream(name)
	}
}

func (s *NATSInstance) Prepare(process *process.ProcessContext, cfg *config.JetStream) (natsclient.JetStreamContext, *natsclient.Conn) {
	natsLock.Lock()
	defer natsLock.Unlock()
	// check if we need an in-process NATS Server
	if len(cfg.Addresses) != 0 {
		return setupNATS(cfg, nil)
	}
	if s.Server == nil {
		var err error
		opts := &natsserver.Options{
			ServerName:      "monolith",
			DontListen:      true,
			JetStream:       true,
			StoreDir:        string(cfg.StoragePath),
			NoSystemAccount: true,
			MaxPayload:      16 * 1024 * 1024,
			NoSigs:          true,
			NoLog:           cfg.NoLog,
		}
		s.Server, err = natsserver.NewServer(opts)
		if err != nil {
			panic(err)
		}
		if !cfg.NoLog {
			s.SetLogger(NewLogAdapter(), opts.Debug, opts.Trace)
		}
		go func() {
			process.ComponentStarted()
			s.Start()
		}()
		go func() {
			<-process.WaitForShutdown()
			s.Shutdown()
			s.WaitForShutdown()
			process.ComponentFinished()
		}()
	}
	if !s.ReadyForConnections(time.Second * 10) {
		logrus.Fatalln("NATS did not start in time")
	}
	// reuse existing connections
	if s.nc != nil {
		return s.js, s.nc
	}
	nc, err := natsclient.Connect("", natsclient.InProcessServer(s))
	if err != nil {
		logrus.Fatalln("Failed to create NATS client")
	}
	js, _ := setupNATS(cfg, nc)
	s.js = js
	s.nc = nc
	return js, nc
}

func setupNATS(cfg *config.JetStream, nc *natsclient.Conn) (natsclient.JetStreamContext, *natsclient.Conn) {
	var s nats.JetStreamContext
	var err error
	if nc == nil {
		opts := []natsclient.Option{
			natsclient.DisconnectErrHandler(func(c *natsclient.Conn, err error) {
				logrus.WithError(err).Error("nats connection: disconnected")
			}),
			natsclient.ReconnectHandler(func(_ *natsclient.Conn) {
				logrus.Info("nats connection: client reconnected")
				for _, stream := range []*nats.StreamConfig{
					streams[6],
					streams[10],
				} {
					err = configureStream(stream, cfg, s)
					if err != nil {
						logrus.WithError(err).WithField("stream", stream.Name).Error("unable to configure a stream")
					}

				}
			}),
			natsclient.ClosedHandler(func(_ *natsclient.Conn) {
				logrus.Info("nats connection: client closed")
			}),
		}
		if cfg.DisableTLSValidation {
			opts = append(opts, natsclient.Secure(&tls.Config{
				InsecureSkipVerify: true,
			}))
		}
		nc, err = natsclient.Connect(strings.Join(cfg.Addresses, ","), opts...)
		if err != nil {
			logrus.WithError(err).Panic("Unable to connect to NATS")
			return nil, nil
		}
	}

	s, err = nc.JetStream()
	if err != nil {
		logrus.WithError(err).Panic("Unable to get JetStream context")
		return nil, nil
	}

	for _, stream := range streams { // streams are defined in streams.go
		err = configureStream(stream, cfg, s)
		if err != nil {
			logrus.WithError(err).WithField("stream", stream.Name).Fatal("unable to configure a stream")
		}
	}

	// Clean up old consumers so that interest-based consumers do the
	// right thing.
	for stream, consumers := range map[string][]string{
		OutputClientData:        {"SyncAPIClientAPIConsumer"},
		OutputReceiptEvent:      {"SyncAPIEDUServerReceiptConsumer", "FederationAPIEDUServerConsumer"},
		OutputSendToDeviceEvent: {"SyncAPIEDUServerSendToDeviceConsumer", "FederationAPIEDUServerConsumer"},
		OutputTypingEvent:       {"SyncAPIEDUServerTypingConsumer", "FederationAPIEDUServerConsumer"},
		OutputRoomEvent:         {"AppserviceRoomserverConsumer"},
		OutputStreamEvent:       {"UserAPISyncAPIStreamEventConsumer"},
		OutputReadUpdate:        {"UserAPISyncAPIReadUpdateConsumer"},
	} {
		streamName := cfg.Matrix.JetStream.Prefixed(stream)
		for _, consumer := range consumers {
			consumerName := cfg.Matrix.JetStream.Prefixed(consumer) + "Pull"
			consumerInfo, err := s.ConsumerInfo(streamName, consumerName)
			if err != nil || consumerInfo == nil {
				continue
			}
			if err = s.DeleteConsumer(streamName, consumerName); err != nil {
				logrus.WithError(err).Errorf("Unable to clean up old consumer %q for stream %q", consumer, stream)
			}
		}
	}

	return s, nc
}

func configureStream(stream *nats.StreamConfig, cfg *config.JetStream, s nats.JetStreamContext) error {
	name := cfg.Prefixed(stream.Name)
	info, err := s.StreamInfo(name)
	if err != nil && err != natsclient.ErrStreamNotFound {
		return fmt.Errorf("get stream info: %w", err)
	}
	subjects := stream.Subjects
	if len(subjects) == 0 {
		// By default we want each stream to listen for the subjects
		// that are either an exact match for the stream name, or where
		// the first part of the subject is the stream name. ">" is a
		// wildcard in NATS for one or more subject tokens. In the case
		// that the stream is called "Foo", this will match any message
		// with the subject "Foo", "Foo.Bar" or "Foo.Bar.Baz" etc.
		subjects = []string{name, name + ".>"}
	}
	if info != nil {
		switch {
		case !reflect.DeepEqual(info.Config.Subjects, subjects):
			fallthrough
		case info.Config.Retention != stream.Retention:
			fallthrough
		case info.Config.Storage != stream.Storage:
			if err = s.DeleteStream(name); err != nil {
				return fmt.Errorf("delete stream: %w", err)
			}
			info = nil
		}
	}
	if info == nil {
		// If we're trying to keep everything in memory (e.g. unit tests)
		// then overwrite the storage policy.
		if cfg.InMemory {
			stream.Storage = natsclient.MemoryStorage
		}

		// Namespace the streams without modifying the original streams
		// array, otherwise we end up with namespaces on namespaces.
		namespaced := *stream
		namespaced.Name = name
		namespaced.Subjects = subjects
		if _, err = s.AddStream(&namespaced); err != nil {
			return fmt.Errorf("add stream: %w", err)
		}
		logrus.Infof("stream created: %s", stream.Name)
	}
	return nil
}
