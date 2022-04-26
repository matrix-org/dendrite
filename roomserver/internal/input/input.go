// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package input contains the code processes new room events
package input

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Arceliar/phony"
	"github.com/getsentry/sentry-go"
	fedapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

var keyContentFields = map[string]string{
	"m.room.join_rules":         "join_rule",
	"m.room.history_visibility": "history_visibility",
	"m.room.member":             "membership",
}

// Inputer is responsible for consuming from the roomserver input
// streams and processing the events. All input events are queued
// into a single NATS stream and the order is preserved strictly.
// The `room_id` message header will contain the room ID which will
// be used to assign the pending event to a per-room worker.
//
// The input API maintains an ephemeral headers-only consumer. It
// will speed through the stream working out which room IDs are
// pending and create durable consumers for them. The durable
// consumer will then be used for each room worker goroutine to
// fetch events one by one and process them. Each room having a
// durable consumer of its own means there is no head-of-line
// blocking between rooms. Filtering ensures that each durable
// consumer only receives events for the room it is interested in.
//
// The ephemeral consumer closely tracks the newest events. The
// per-room durable consumers will only progress through the stream
// as events are processed.
//
//       A BC *  -> positions of each consumer (* = ephemeral)
//       ⌄ ⌄⌄ ⌄
// ABAABCAABCAA  -> newest (letter = subject for each message)
//
// In this example, A is still processing an event but has two
// pending events to process afterwards. Both B and C are caught
// up, so they will do nothing until a new event comes in for B
// or C.
type Inputer struct {
	Cfg                  *config.RoomServer
	ProcessContext       *process.ProcessContext
	DB                   storage.Database
	NATSClient           *nats.Conn
	JetStream            nats.JetStreamContext
	Durable              nats.SubOpt
	ServerName           gomatrixserverlib.ServerName
	FSAPI                fedapi.FederationInternalAPI
	KeyRing              gomatrixserverlib.JSONVerifier
	ACLs                 *acls.ServerACLs
	InputRoomEventTopic  string
	OutputRoomEventTopic string
	workers              sync.Map // room ID -> *worker

	Queryer *query.Queryer
}

type worker struct {
	phony.Inbox
	sync.Mutex
	r            *Inputer
	roomID       string
	subscription *nats.Subscription
}

func (r *Inputer) startWorkerForRoom(roomID string) {
	v, loaded := r.workers.LoadOrStore(roomID, &worker{
		r:      r,
		roomID: roomID,
	})
	w := v.(*worker)
	w.Lock()
	defer w.Unlock()
	if !loaded || w.subscription == nil {
		consumer := r.Cfg.Matrix.JetStream.Prefixed("RoomInput" + jetstream.Tokenise(w.roomID))
		subject := r.Cfg.Matrix.JetStream.Prefixed(jetstream.InputRoomEventSubj(w.roomID))

		// Create the consumer. We do this as a specific step rather than
		// letting PullSubscribe create it for us because we need the consumer
		// to outlive the subscription. If we do it this way, we can Bind in the
		// next step, and when we Unsubscribe, the consumer continues to live. If
		// we leave PullSubscribe to create the durable consumer, Unsubscribe will
		// delete it because it thinks it "owns" it, which in turn breaks the
		// interest-based retention storage policy.
		// If the durable consumer already exists, this is effectively a no-op.
		// Another interesting tid-bit here: the ACK policy is set to "all" so that
		// if we acknowledge a message, we also acknowledge everything that comes
		// before it. This is necessary because otherwise our consumer will never
		// acknowledge things we filtered out for other subjects and therefore they
		// will linger around forever.
		if _, err := w.r.JetStream.AddConsumer(
			r.Cfg.Matrix.JetStream.Prefixed(jetstream.InputRoomEvent),
			&nats.ConsumerConfig{
				Durable:       consumer,
				AckPolicy:     nats.AckAllPolicy,
				DeliverPolicy: nats.DeliverAllPolicy,
				FilterSubject: subject,
				AckWait:       MaximumMissingProcessingTime + (time.Second * 10),
			},
		); err != nil {
			logrus.WithError(err).Errorf("Failed to create consumer for room %q", w.roomID)
			return
		}

		// Bind to our durable consumer. We want to receive all messages waiting
		// for this subject and we want to manually acknowledge them, so that we
		// can ensure they are only cleaned up when we are done processing them.
		sub, err := w.r.JetStream.PullSubscribe(
			subject, consumer,
			nats.ManualAck(),
			nats.DeliverAll(),
			nats.AckWait(MaximumMissingProcessingTime+(time.Second*10)),
			nats.Bind(r.InputRoomEventTopic, consumer),
		)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to subscribe to stream for room %q", w.roomID)
			return
		}

		// Go and start pulling messages off the queue.
		w.subscription = sub
		w.Act(nil, w._next)
	}
}

// Start creates an ephemeral non-durable consumer on the roomserver
// input topic. It is configured to deliver us headers only because we
// don't actually care about the contents of the message at this point,
// we only care about the `room_id` field. Once a message arrives, we
// will look to see if we have a worker for that room which has its
// own consumer. If we don't, we'll start one.
func (r *Inputer) Start() error {
	prometheus.MustRegister(roomserverInputBackpressure, processRoomEventDuration)
	_, err := r.JetStream.Subscribe(
		"", // This is blank because we specified it in BindStream.
		func(m *nats.Msg) {
			roomID := m.Header.Get(jetstream.RoomID)
			r.startWorkerForRoom(roomID)
			_ = m.Ack()
		},
		nats.HeadersOnly(),
		nats.DeliverAll(),
		nats.AckAll(),
		nats.BindStream(r.InputRoomEventTopic),
	)
	return err
}

// _next is called by the worker for the room. It must only be called
// by the actor embedded into the worker.
func (w *worker) _next() {
	// Look up what the next event is that's waiting to be processed.
	ctx, cancel := context.WithTimeout(w.r.ProcessContext.Context(), time.Minute)
	defer cancel()
	msgs, err := w.subscription.Fetch(1, nats.Context(ctx))
	switch err {
	case nil:
		// Make sure that once we're done here, we queue up another call
		// to _next in the inbox.
		defer w.Act(nil, w._next)

		// If no error was reported, but we didn't get exactly one message,
		// then skip over this and try again on the next iteration.
		if len(msgs) != 1 {
			return
		}

	case context.DeadlineExceeded:
		// The context exceeded, so we've been waiting for more than a
		// minute for activity in this room. At this point we will shut
		// down the subscriber to free up resources. It'll get started
		// again if new activity happens.
		if err = w.subscription.Unsubscribe(); err != nil {
			logrus.WithError(err).Errorf("Failed to unsubscribe to stream for room %q", w.roomID)
		}
		w.Lock()
		w.subscription = nil
		w.Unlock()
		return

	default:
		// Something went wrong while trying to fetch the next event
		// from the queue. In which case, we'll shut down the subscriber
		// and wait to be notified about new room activity again. Maybe
		// the problem will be corrected by then.
		logrus.WithError(err).Errorf("Failed to get next stream message for room %q", w.roomID)
		if err = w.subscription.Unsubscribe(); err != nil {
			logrus.WithError(err).Errorf("Failed to unsubscribe to stream for room %q", w.roomID)
		}
		w.Lock()
		w.subscription = nil
		w.Unlock()
		return
	}

	// Try to unmarshal the input room event. If the JSON unmarshalling
	// fails then we'll terminate the message — this notifies NATS that
	// we are done with the message and never want to see it again.
	msg := msgs[0]
	var inputRoomEvent api.InputRoomEvent
	if err = json.Unmarshal(msg.Data, &inputRoomEvent); err != nil {
		_ = msg.Term()
		return
	}

	roomserverInputBackpressure.With(prometheus.Labels{"room_id": w.roomID}).Inc()
	defer roomserverInputBackpressure.With(prometheus.Labels{"room_id": w.roomID}).Dec()

	// Process the room event. If something goes wrong then we'll tell
	// NATS to terminate the message. We'll store the error result as
	// a string, because we might want to return that to the caller if
	// it was a synchronous request.
	var errString string
	if err = w.r.processRoomEvent(w.r.ProcessContext.Context(), &inputRoomEvent); err != nil {
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			sentry.CaptureException(err)
		}
		logrus.WithError(err).WithFields(logrus.Fields{
			"room_id":  w.roomID,
			"event_id": inputRoomEvent.Event.EventID(),
			"type":     inputRoomEvent.Event.Type(),
		}).Warn("Roomserver failed to process async event")
		_ = msg.Term()
		errString = err.Error()
	} else {
		_ = msg.Ack()
	}

	// If it was a synchronous input request then the "sync" field
	// will be present in the message. That means that someone is
	// waiting for a response. The temporary inbox name is present in
	// that field, so send back the error string (if any). If there
	// was no error then we'll return a blank message, which means
	// that everything was OK.
	if replyTo := msg.Header.Get("sync"); replyTo != "" {
		if err = w.r.NATSClient.Publish(replyTo, []byte(errString)); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"room_id":  w.roomID,
				"event_id": inputRoomEvent.Event.EventID(),
				"type":     inputRoomEvent.Event.Type(),
			}).Warn("Roomserver failed to respond for sync event")
		}
	}
}

// queueInputRoomEvents queues events into the roomserver input
// stream in NATS.
func (r *Inputer) queueInputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
) (replySub *nats.Subscription, err error) {
	// If the request is synchronous then we need to create a
	// temporary inbox to wait for responses on, and then create
	// a subscription to it. If it's asynchronous then we won't
	// bother, so these values will remain empty.
	var replyTo string
	if !request.Asynchronous {
		replyTo = nats.NewInbox()
		replySub, err = r.NATSClient.SubscribeSync(replyTo)
		if err != nil {
			return nil, fmt.Errorf("r.NATSClient.SubscribeSync: %w", err)
		}
		if replySub == nil {
			// This shouldn't ever happen, but it doesn't hurt to check
			// because we can potentially avoid a nil pointer panic later
			// if it did for some reason.
			return nil, fmt.Errorf("expected a subscription to the temporary inbox")
		}
	}

	// For each event, marshal the input room event and then
	// send it into the input queue.
	for _, e := range request.InputRoomEvents {
		roomID := e.Event.RoomID()
		subj := r.Cfg.Matrix.JetStream.Prefixed(jetstream.InputRoomEventSubj(roomID))
		msg := &nats.Msg{
			Subject: subj,
			Header:  nats.Header{},
		}
		msg.Header.Set("room_id", roomID)
		if replyTo != "" {
			msg.Header.Set("sync", replyTo)
		}
		msg.Data, err = json.Marshal(e)
		if err != nil {
			return nil, fmt.Errorf("json.Marshal: %w", err)
		}
		if _, err = r.JetStream.PublishMsg(msg, nats.Context(ctx)); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"room_id":  roomID,
				"event_id": e.Event.EventID(),
				"subj":     subj,
			}).Error("Roomserver failed to queue async event")
			return nil, fmt.Errorf("r.JetStream.PublishMsg: %w", err)
		}
	}
	return
}

// InputRoomEvents implements api.RoomserverInternalAPI
func (r *Inputer) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
	// Queue up the event into the roomserver.
	replySub, err := r.queueInputRoomEvents(ctx, request)
	if err != nil {
		response.ErrMsg = err.Error()
		return
	}

	// If we aren't waiting for synchronous responses then we can
	// give up here, there is nothing further to do.
	if replySub == nil {
		return
	}

	// Otherwise, we'll want to sit and wait for the responses
	// from the roomserver. There will be one response for every
	// input we submitted. The last error value we receive will
	// be the one returned as the error string.
	defer replySub.Drain() // nolint:errcheck
	for i := 0; i < len(request.InputRoomEvents); i++ {
		msg, err := replySub.NextMsgWithContext(ctx)
		if err != nil {
			response.ErrMsg = err.Error()
			return
		}
		if len(msg.Data) > 0 {
			response.ErrMsg = string(msg.Data)
		}
	}
}

// WriteOutputEvents implements OutputRoomEventWriter
func (r *Inputer) WriteOutputEvents(roomID string, updates []api.OutputEvent) error {
	var err error
	for _, update := range updates {
		msg := &nats.Msg{
			Subject: r.OutputRoomEventTopic,
			Header:  nats.Header{},
		}
		msg.Header.Set(jetstream.RoomID, roomID)
		msg.Data, err = json.Marshal(update)
		if err != nil {
			return err
		}
		logger := log.WithFields(log.Fields{
			"room_id": roomID,
			"type":    update.Type,
		})
		if update.NewRoomEvent != nil {
			eventType := update.NewRoomEvent.Event.Type()
			logger = logger.WithFields(log.Fields{
				"event_type":     eventType,
				"event_id":       update.NewRoomEvent.Event.EventID(),
				"adds_state":     len(update.NewRoomEvent.AddsStateEventIDs),
				"removes_state":  len(update.NewRoomEvent.RemovesStateEventIDs),
				"send_as_server": update.NewRoomEvent.SendAsServer,
				"sender":         update.NewRoomEvent.Event.Sender(),
			})
			if update.NewRoomEvent.Event.StateKey() != nil {
				logger = logger.WithField("state_key", *update.NewRoomEvent.Event.StateKey())
			}
			contentKey := keyContentFields[eventType]
			if contentKey != "" {
				value := gjson.GetBytes(update.NewRoomEvent.Event.Content(), contentKey)
				if value.Exists() {
					logger = logger.WithField("content_value", value.String())
				}
			}

			if eventType == "m.room.server_acl" && update.NewRoomEvent.Event.StateKeyEquals("") {
				ev := update.NewRoomEvent.Event.Unwrap()
				defer r.ACLs.OnServerACLUpdate(ev)
			}
		}
		logger.Tracef("Producing to topic '%s'", r.OutputRoomEventTopic)
		if _, err := r.JetStream.PublishMsg(msg); err != nil {
			logger.WithError(err).Errorf("Failed to produce to topic '%s': %s", r.OutputRoomEventTopic, err)
			return err
		}
	}
	return nil
}

var roomserverInputBackpressure = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "dendrite",
		Subsystem: "roomserver",
		Name:      "input_backpressure",
		Help:      "How many events are queued for input for a given room",
	},
	[]string{"room_id"},
)
