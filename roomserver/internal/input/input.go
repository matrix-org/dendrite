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
	if !loaded {
		consumer := "DendriteRoomInputConsumerPull" + jetstream.Tokenise(w.roomID)
		subject := jetstream.InputRoomEventSubj(w.roomID)

		if info, err := w.r.JetStream.ConsumerInfo(
			jetstream.InputRoomEvent,
			consumer,
		); err != nil || info == nil {
			if _, err := w.r.JetStream.AddConsumer(
				r.Cfg.Matrix.JetStream.TopicFor(jetstream.InputRoomEvent),
				&nats.ConsumerConfig{
					Durable:       consumer,
					AckPolicy:     nats.AckExplicitPolicy,
					DeliverPolicy: nats.DeliverAllPolicy,
					FilterSubject: subject,
					AckWait:       MaximumMissingProcessingTime + (time.Second * 10),
				},
			); err != nil {
				logrus.WithError(err).Errorf("Failed to create consumer for room %q", w.roomID)
				return
			}
		}

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

		logrus.Infof("Started stream for room %q", w.roomID)
		w.subscription = sub
		w.Act(nil, w.next)
	}
}

// onMessage is called when a new event arrives in the roomserver input stream.
func (r *Inputer) Start() error {
	_, err := r.JetStream.Subscribe(
		"", // don't supply a subject because we're using BindStream
		func(m *nats.Msg) {
			roomID := m.Header.Get(jetstream.RoomID)
			r.startWorkerForRoom(roomID)
		},
		nats.DeliverAll(),
		nats.AckNone(),
		nats.BindStream(r.InputRoomEventTopic),
	)
	return err
}

func (w *worker) next() {
	ctx, cancel := context.WithTimeout(w.r.ProcessContext.Context(), time.Second*30)
	defer cancel()
	msgs, err := w.subscription.Fetch(1, nats.Context(ctx))
	switch err {
	case nil:
		if len(msgs) != 1 {
			return
		}
		defer w.Act(nil, w.next)
	case context.DeadlineExceeded:
		logrus.Infof("Stream for room %q idle, shutting down", w.roomID)
		if err = w.subscription.Unsubscribe(); err != nil {
			logrus.WithError(err).Errorf("Failed to unsubscribe to stream for room %q", w.roomID)
		}
		w.r.workers.Delete(w.roomID)
		return
	case nats.ErrTimeout:
		w.Act(nil, w.next)
		return
	default:
		logrus.WithError(err).Errorf("Failed to get next stream message for room %q", w.roomID)
		if err = w.subscription.Unsubscribe(); err != nil {
			logrus.WithError(err).Errorf("Failed to unsubscribe to stream for room %q", w.roomID)
		}
		w.r.workers.Delete(w.roomID)
		return
	}

	msg := msgs[0]
	var inputRoomEvent api.InputRoomEvent
	if err = json.Unmarshal(msg.Data, &inputRoomEvent); err != nil {
		_ = msg.Term()
		return
	}

	roomserverInputBackpressure.With(prometheus.Labels{"room_id": w.roomID}).Inc()
	defer roomserverInputBackpressure.With(prometheus.Labels{"room_id": w.roomID}).Dec()

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

// InputRoomEvents implements api.RoomserverInternalAPI
func (r *Inputer) InputRoomEvents(
	ctx context.Context,
	request *api.InputRoomEventsRequest,
	response *api.InputRoomEventsResponse,
) {
	var replyTo string
	var replySub *nats.Subscription
	if !request.Asynchronous {
		var err error
		replyTo = nats.NewInbox()
		replySub, err = r.NATSClient.SubscribeSync(replyTo)
		if err != nil {
			response.ErrMsg = err.Error()
			return
		}
	}

	var err error
	for _, e := range request.InputRoomEvents {
		roomID := e.Event.RoomID()
		subj := jetstream.InputRoomEventSubj(roomID)
		msg := &nats.Msg{
			Subject: subj,
			Header:  nats.Header{},
			Reply:   replyTo,
		}
		msg.Header.Set("room_id", roomID)
		if replyTo != "" {
			msg.Header.Set("sync", replyTo)
		}
		msg.Data, err = json.Marshal(e)
		if err != nil {
			response.ErrMsg = err.Error()
			return
		}
		if _, err = r.JetStream.PublishMsg(msg); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"room_id":  roomID,
				"event_id": e.Event.EventID(),
				"subj":     subj,
			}).Error("Roomserver failed to queue async event")
			return
		}
	}

	if request.Asynchronous || replySub == nil {
		return
	}

	defer replySub.Drain() // nolint:errcheck
	for i := 0; i < len(request.InputRoomEvents); i++ {
		msg, err := replySub.NextMsgWithContext(ctx)
		if err != nil {
			response.ErrMsg = err.Error()
			return
		}
		if len(msg.Data) > 0 {
			response.ErrMsg = string(msg.Data)
			return
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
