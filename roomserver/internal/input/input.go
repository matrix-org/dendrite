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
	"sync"
	"time"

	"github.com/Arceliar/phony"
	"github.com/getsentry/sentry-go"
	fedapi "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/query"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/jetstream"
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

type Inputer struct {
	DB                   storage.Database
	JetStream            nats.JetStreamContext
	Durable              nats.SubOpt
	ServerName           gomatrixserverlib.ServerName
	FSAPI                fedapi.FederationInternalAPI
	KeyRing              gomatrixserverlib.JSONVerifier
	ACLs                 *acls.ServerACLs
	InputRoomEventTopic  string
	OutputRoomEventTopic string
	workers              sync.Map // room ID -> *phony.Inbox

	Queryer *query.Queryer
}

func (r *Inputer) workerForRoom(roomID string) *phony.Inbox {
	inbox, _ := r.workers.LoadOrStore(roomID, &phony.Inbox{})
	return inbox.(*phony.Inbox)
}

// eventsInProgress is an in-memory map to keep a track of which events we have
// queued up for processing. If we get a redelivery from NATS and we still have
// the queued up item then we won't do anything with the redelivered message. If
// we've restarted Dendrite and now this map is empty then it means that we will
// reload pending work from NATS.
var eventsInProgress sync.Map

// onMessage is called when a new event arrives in the roomserver input stream.
func (r *Inputer) Start() error {
	_, err := r.JetStream.Subscribe(
		r.InputRoomEventTopic,
		// We specifically don't use jetstream.WithJetStreamMessage here because we
		// queue the task off to a room-specific queue and the ACK needs to be sent
		// later, possibly with an error response to the inputter if synchronous.
		func(msg *nats.Msg) {
			roomID := msg.Header.Get("room_id")
			var inputRoomEvent api.InputRoomEvent
			if err := json.Unmarshal(msg.Data, &inputRoomEvent); err != nil {
				_ = msg.Term()
				return
			}

			_ = msg.InProgress()
			index := roomID + "\000" + inputRoomEvent.Event.EventID()
			if _, ok := eventsInProgress.LoadOrStore(index, struct{}{}); ok {
				// We're already waiting to deal with this event, so there's no
				// point in queuing it up again. We've notified NATS that we're
				// working on the message still, so that will have deferred the
				// redelivery by a bit.
				return
			}

			roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Inc()
			r.workerForRoom(roomID).Act(nil, func() {
				_ = msg.InProgress() // resets the acknowledgement wait timer
				defer eventsInProgress.Delete(index)
				defer roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Dec()
				if err := r.processRoomEvent(context.Background(), &inputRoomEvent); err != nil {
					if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
						sentry.CaptureException(err)
					}
					logrus.WithError(err).WithFields(logrus.Fields{
						"room_id":  roomID,
						"event_id": inputRoomEvent.Event.EventID(),
						"type":     inputRoomEvent.Event.Type(),
					}).Warn("Roomserver failed to process async event")
				}
				_ = msg.Ack()
			})
		},
		// NATS wants to acknowledge automatically by default when the message is
		// read from the stream, but we want to override that behaviour by making
		// sure that we only acknowledge when we're happy we've done everything we
		// can. This ensures we retry things when it makes sense to do so.
		nats.ManualAck(),
		// Use a durable named consumer.
		r.Durable,
		// If we've missed things in the stream, e.g. we restarted, then replay
		// all of the queued messages that were waiting for us.
		nats.DeliverAll(),
		// Ensure that NATS doesn't try to resend us something that wasn't done
		// within the period of time that we might still be processing it.
		nats.AckWait(MaximumMissingProcessingTime+(time.Second*10)),
	)
	return err
}

// InputRoomEvents implements api.RoomserverInternalAPI
func (r *Inputer) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
	if request.Asynchronous {
		var err error
		for _, e := range request.InputRoomEvents {
			msg := &nats.Msg{
				Subject: r.InputRoomEventTopic,
				Header:  nats.Header{},
			}
			roomID := e.Event.RoomID()
			msg.Header.Set("room_id", roomID)
			msg.Data, err = json.Marshal(e)
			if err != nil {
				response.ErrMsg = err.Error()
				return
			}
			if _, err = r.JetStream.PublishMsg(msg); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"room_id":  roomID,
					"event_id": e.Event.EventID(),
				}).Error("Roomserver failed to queue async event")
				return
			}
		}
	} else {
		responses := make(chan error, len(request.InputRoomEvents))
		for _, e := range request.InputRoomEvents {
			inputRoomEvent := e
			roomID := inputRoomEvent.Event.RoomID()
			index := roomID + "\000" + inputRoomEvent.Event.EventID()
			if _, ok := eventsInProgress.LoadOrStore(index, struct{}{}); ok {
				// We're already waiting to deal with this event, so there's no
				// point in queuing it up again. We've notified NATS that we're
				// working on the message still, so that will have deferred the
				// redelivery by a bit.
				return
			}
			roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Inc()
			worker := r.workerForRoom(roomID)
			worker.Act(nil, func() {
				defer eventsInProgress.Delete(index)
				defer roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Dec()
				err := r.processRoomEvent(ctx, &inputRoomEvent)
				if err != nil {
					if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
						sentry.CaptureException(err)
					}
					logrus.WithError(err).WithFields(logrus.Fields{
						"room_id":  roomID,
						"event_id": inputRoomEvent.Event.EventID(),
					}).Warn("Roomserver failed to process sync event")
				}
				select {
				case <-ctx.Done():
				default:
					responses <- err
				}
			})
		}
		for i := 0; i < len(request.InputRoomEvents); i++ {
			select {
			case <-ctx.Done():
				response.ErrMsg = context.DeadlineExceeded.Error()
				return
			case err := <-responses:
				if err != nil {
					response.ErrMsg = err.Error()
					return
				}
			}
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

func init() {
	prometheus.MustRegister(roomserverInputBackpressure)
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
