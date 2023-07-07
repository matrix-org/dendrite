package jetstream

import (
	"crypto/tls"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"

	natsserver "github.com/nats-io/nats-server/v2/server"
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
		return setupNATS(process, cfg, nil)
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
	if !s.ReadyForConnections(time.Second * 60) {
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
	js, _ := setupNATS(process, cfg, nc)
	s.js = js
	s.nc = nc
	return js, nc
}

// nolint:gocyclo
func setupNATS(process *process.ProcessContext, cfg *config.JetStream, nc *natsclient.Conn) (natsclient.JetStreamContext, *natsclient.Conn) {
	if nc == nil {
		var err error
		opts := []natsclient.Option{}
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
			// If the stream config doesn't match what we expect, try to update
			// it. If that doesn't work then try to blow it away and we'll then
			// recreate it in the next section.
			// Each specific option that we set must be checked by hand, as if
			// you DeepEqual the whole config struct, it will always show that
			// there's a difference because the NATS Server will return defaults
			// in the stream info.
			switch {
			case !reflect.DeepEqual(info.Config.Subjects, subjects):
				fallthrough
			case info.Config.Retention != stream.Retention:
				fallthrough
			case info.Config.Storage != stream.Storage:
				fallthrough
			case info.Config.MaxAge != stream.MaxAge:
				// Try updating the stream first, as many things can be updated
				// non-destructively.
				if info, err = s.UpdateStream(stream); err != nil {
					logrus.WithError(err).Warnf("Unable to update stream %q, recreating...", name)
					// We failed to update the stream, this is a last attempt to get
					// things working but may result in data loss.
					if err = s.DeleteStream(name); err != nil {
						logrus.WithError(err).Fatalf("Unable to delete stream %q", name)
					}
					info = nil
				}
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
					err := fmt.Errorf("Stream %q is running in-memory; this may be due to data corruption in the JetStream storage directory", namespaced.Name)
					sentry.CaptureException(err)
					process.Degraded(err)
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
