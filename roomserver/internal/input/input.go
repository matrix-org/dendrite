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
	"sync"

	"github.com/Arceliar/phony"
	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
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
	ACLs                 *acls.ServerACLs
	InputRoomEventTopic  string
	OutputRoomEventTopic string
	workers              sync.Map // room ID -> *phony.Inbox
}

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
			inbox, _ := r.workers.LoadOrStore(roomID, &phony.Inbox{})
			roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Inc()
			inbox.(*phony.Inbox).Act(nil, func() {
				defer roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Dec()
				if err := r.processRoomEvent(context.TODO(), &inputRoomEvent); err != nil {
					sentry.CaptureException(err)
				} else {
					hooks.Run(hooks.KindNewEventPersisted, inputRoomEvent.Event)
				}
				_ = msg.Ack()
			})
		},
		// NATS wants to acknowledge automatically by default when the message is
		// read from the stream, but we want to override that behaviour by making
		// sure that we only acknowledge when we're happy we've done everything we
		// can. This ensures we retry things when it makes sense to do so.
		nats.ManualAck(),
		// NATS will try to redeliver things to us automatically if we don't ack
		// or nak them within a certain amount of time. This stops that from
		// happening, so we don't end up doing a lot of unnecessary duplicate work.
		nats.MaxDeliver(0),
		// Use a durable named consumer.
		r.Durable,
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
				return
			}
		}
	} else {
		responses := make(chan error, len(request.InputRoomEvents))
		defer close(responses)
		for _, e := range request.InputRoomEvents {
			inputRoomEvent := e
			roomID := inputRoomEvent.Event.RoomID()
			inbox, _ := r.workers.LoadOrStore(roomID, &phony.Inbox{})
			roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Inc()
			inbox.(*phony.Inbox).Act(nil, func() {
				defer roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Dec()
				err := r.processRoomEvent(context.TODO(), &inputRoomEvent)
				if err != nil {
					sentry.CaptureException(err)
				} else {
					hooks.Run(hooks.KindNewEventPersisted, inputRoomEvent.Event)
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
