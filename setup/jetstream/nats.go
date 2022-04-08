package jetstream

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/sirupsen/logrus"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	natsclient "github.com/nats-io/nats.go"
)

var natsServer *natsserver.Server
var natsServerMutex sync.Mutex

func PrepareForTests() (*process.ProcessContext, nats.JetStreamContext, *nats.Conn) {
	cfg := &config.Dendrite{}
	cfg.Defaults(true)
	cfg.Global.JetStream.InMemory = true
	pc := process.NewProcessContext()
	js, jc := Prepare(pc, &cfg.Global.JetStream)
	return pc, js, jc
}

func Prepare(process *process.ProcessContext, cfg *config.JetStream) (natsclient.JetStreamContext, *natsclient.Conn) {
	// check if we need an in-process NATS Server
	if len(cfg.Addresses) != 0 {
		return setupNATS(process, cfg, nil)
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
		go func() {
			process.ComponentStarted()
			natsServer.Start()
		}()
		go func() {
			<-process.WaitForShutdown()
			natsServer.Shutdown()
			natsServer.WaitForShutdown()
			process.ComponentFinished()
		}()
	}
	natsServerMutex.Unlock()
	if !natsServer.ReadyForConnections(time.Second * 10) {
		logrus.Fatalln("NATS did not start in time")
	}
	nc, err := natsclient.Connect("", natsclient.InProcessServer(natsServer))
	if err != nil {
		logrus.Fatalln("Failed to create NATS client")
	}
	return setupNATS(process, cfg, nc)
}

func setupNATS(process *process.ProcessContext, cfg *config.JetStream, nc *natsclient.Conn) (natsclient.JetStreamContext, *natsclient.Conn) {
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
		name := cfg.Prefixed(stream.Name)
		info, err := s.StreamInfo(name)
		if err != nil && err != natsclient.ErrStreamNotFound {
			logrus.WithError(err).Fatal("Unable to get stream info")
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
					logrus.WithError(err).Fatal("Unable to delete stream")
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
				logger := logrus.WithError(err).WithFields(logrus.Fields{
					"stream":   namespaced.Name,
					"subjects": namespaced.Subjects,
				})

				// If the stream was supposed to be in-memory to begin with
				// then an error here is fatal so we'll give up.
				if namespaced.Storage == natsclient.MemoryStorage {
					logger.WithError(err).Fatal("Unable to add in-memory stream")
				}

				// The stream was supposed to be on disk. Let's try starting
				// Dendrite with the stream in-memory instead. That'll mean that
				// we can't recover anything that was queued on the disk but we
				// will still be able to start and run hopefully in the meantime.
				logger.WithError(err).Error("Unable to add stream")
				sentry.CaptureException(fmt.Errorf("Unable to add stream %q: %w", namespaced.Name, err))

				namespaced.Storage = natsclient.MemoryStorage
				if _, err = s.AddStream(&namespaced); err != nil {
					// We tried to add the stream in-memory instead but something
					// went wrong. That's an unrecoverable situation so we will
					// give up at this point.
					logger.WithError(err).Fatal("Unable to add in-memory stream")
				}

				if stream.Storage != namespaced.Storage {
					// We've managed to add the stream in memory.  What's on the
					// disk will be left alone, but our ability to recover from a
					// future crash will be limited. Yell about it.
					sentry.CaptureException(fmt.Errorf("Stream %q is running in-memory; this may be due to data corruption in the JetStream storage directory, investigate as soon as possible", namespaced.Name))
					logrus.Warn("Stream is running in-memory; this may be due to data corruption in the JetStream storage directory, investigate as soon as possible")
					process.Degraded()
				}
			}
		}
	}

	// Clean up old consumers so that interest-based consumers do the
	// right thing.
	for stream, consumers := range map[string][]string{
		OutputClientData:        {"SyncAPIClientAPIConsumer"},
		OutputReceiptEvent:      {"SyncAPIEDUServerReceiptConsumer", "FederationAPIEDUServerConsumer"},
		OutputSendToDeviceEvent: {"SyncAPIEDUServerSendToDeviceConsumer", "FederationAPIEDUServerConsumer"},
		OutputTypingEvent:       {"SyncAPIEDUServerTypingConsumer", "FederationAPIEDUServerConsumer"},
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
