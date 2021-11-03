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
	"github.com/Shopify/sarama"
	"github.com/getsentry/sentry-go"
	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
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
	Consumer             nats.JetStreamContext
	Producer             sarama.SyncProducer
	ServerName           gomatrixserverlib.ServerName
	ACLs                 *acls.ServerACLs
	InputRoomEventTopic  string
	OutputRoomEventTopic string
	workers              sync.Map // room ID -> *phony.Inbox
}

// onMessage is called when a new event arrives in the roomserver input stream.
func (r *Inputer) Start() error {
	_, err := r.Consumer.Subscribe(
		r.InputRoomEventTopic,
		func(msg *nats.Msg) {
			_ = msg.InProgress()
			roomID := msg.Header.Get("room_id")
			defer roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Dec()
			var inputRoomEvent api.InputRoomEvent
			if err := json.Unmarshal(msg.Data, &inputRoomEvent); err != nil {
				_ = msg.Nak()
				return
			}
			inbox, _ := r.workers.LoadOrStore(roomID, &phony.Inbox{})
			inbox.(*phony.Inbox).Act(nil, func() {
				if _, err := r.processRoomEvent(context.TODO(), &inputRoomEvent); err != nil {
					sentry.CaptureException(err)
					_ = msg.Nak()
				} else {
					hooks.Run(hooks.KindNewEventPersisted, inputRoomEvent.Event)
					_ = msg.Ack()
				}
			})
		},
		nats.ManualAck(),
	)
	return err
}

// InputRoomEvents implements api.RoomserverInternalAPI
func (r *Inputer) InputRoomEvents(
	_ context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
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
		if _, err = r.Consumer.PublishMsg(msg); err != nil {
			response.ErrMsg = err.Error()
			return
		}
		roomserverInputBackpressure.With(prometheus.Labels{"room_id": roomID}).Inc()
	}
}

// WriteOutputEvents implements OutputRoomEventWriter
func (r *Inputer) WriteOutputEvents(roomID string, updates []api.OutputEvent) error {
	messages := make([]*sarama.ProducerMessage, len(updates))
	for i := range updates {
		value, err := json.Marshal(updates[i])
		if err != nil {
			return err
		}
		logger := log.WithFields(log.Fields{
			"room_id": roomID,
			"type":    updates[i].Type,
		})
		if updates[i].NewRoomEvent != nil {
			eventType := updates[i].NewRoomEvent.Event.Type()
			logger = logger.WithFields(log.Fields{
				"event_type":     eventType,
				"event_id":       updates[i].NewRoomEvent.Event.EventID(),
				"adds_state":     len(updates[i].NewRoomEvent.AddsStateEventIDs),
				"removes_state":  len(updates[i].NewRoomEvent.RemovesStateEventIDs),
				"send_as_server": updates[i].NewRoomEvent.SendAsServer,
				"sender":         updates[i].NewRoomEvent.Event.Sender(),
			})
			if updates[i].NewRoomEvent.Event.StateKey() != nil {
				logger = logger.WithField("state_key", *updates[i].NewRoomEvent.Event.StateKey())
			}
			contentKey := keyContentFields[eventType]
			if contentKey != "" {
				value := gjson.GetBytes(updates[i].NewRoomEvent.Event.Content(), contentKey)
				if value.Exists() {
					logger = logger.WithField("content_value", value.String())
				}
			}

			if eventType == "m.room.server_acl" && updates[i].NewRoomEvent.Event.StateKeyEquals("") {
				ev := updates[i].NewRoomEvent.Event.Unwrap()
				defer r.ACLs.OnServerACLUpdate(ev)
			}
		}
		logger.Infof("Producing to topic '%s'", r.OutputRoomEventTopic)
		messages[i] = &sarama.ProducerMessage{
			Topic: r.OutputRoomEventTopic,
			Key:   sarama.StringEncoder(roomID),
			Value: sarama.ByteEncoder(value),
		}
	}
	errs := r.Producer.SendMessages(messages)
	if errs != nil {
		for _, err := range errs.(sarama.ProducerErrors) {
			log.WithError(err).WithField("message_bytes", err.Msg.Value.Length()).Error("Write to kafka failed")
		}
	}
	return errs
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
