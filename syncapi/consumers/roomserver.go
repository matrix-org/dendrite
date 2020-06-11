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

package consumers

import (
	"context"
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// OutputRoomEventConsumer consumes events that originated in the room server.
type OutputRoomEventConsumer struct {
	rsAPI      api.RoomserverInternalAPI
	rsConsumer *internal.ContinualConsumer
	db         storage.Database
	notifier   *sync.Notifier
}

// NewOutputRoomEventConsumer creates a new OutputRoomEventConsumer. Call Start() to begin consuming from room servers.
func NewOutputRoomEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	n *sync.Notifier,
	store storage.Database,
	rsAPI api.RoomserverInternalAPI,
) *OutputRoomEventConsumer {

	consumer := internal.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputRoomEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEventConsumer{
		rsConsumer: &consumer,
		db:         store,
		notifier:   n,
		rsAPI:      rsAPI,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from room servers
func (s *OutputRoomEventConsumer) Start() error {
	return s.rsConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output api.OutputEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	switch output.Type {
	case api.OutputTypeNewRoomEvent:
		return s.onNewRoomEvent(context.TODO(), *output.NewRoomEvent)
	case api.OutputTypeNewInviteEvent:
		return s.onNewInviteEvent(context.TODO(), *output.NewInviteEvent)
	case api.OutputTypeRetireInviteEvent:
		return s.onRetireInviteEvent(context.TODO(), *output.RetireInviteEvent)
	default:
		log.WithField("type", output.Type).Debug(
			"roomserver output log: ignoring unknown output type",
		)
		return nil
	}
}

func (s *OutputRoomEventConsumer) onNewRoomEvent(
	ctx context.Context, msg api.OutputNewRoomEvent,
) error {
	ev := msg.Event
	log.WithFields(log.Fields{
		"event_id":     ev.EventID(),
		"room_id":      ev.RoomID(),
		"room_version": ev.RoomVersion,
	}).Info("received event from roomserver")

	addsStateEvents := []gomatrixserverlib.HeaderedEvent{ev}
	addsStateEvents = append(addsStateEvents, msg.AddStateEvents...)

	ev, err := s.updateStateEvent(ev)
	if err != nil {
		return err
	}

	for i := range addsStateEvents {
		addsStateEvents[i], err = s.updateStateEvent(addsStateEvents[i])
		if err != nil {
			return err
		}
	}

	pduPos, err := s.db.WriteEvent(
		ctx,
		&ev,
		addsStateEvents,
		msg.AddsStateEventIDs,
		msg.RemovesStateEventIDs,
		msg.TransactionID,
		false,
	)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
			"add":        msg.AddsStateEventIDs,
			"del":        msg.RemovesStateEventIDs,
		}).Panicf("roomserver output log: write event failure")
		return nil
	}
	s.notifier.OnNewEvent(&ev, "", nil, types.NewStreamToken(pduPos, 0))

	return nil
}

func (s *OutputRoomEventConsumer) onNewInviteEvent(
	ctx context.Context, msg api.OutputNewInviteEvent,
) error {
	pduPos, err := s.db.AddInviteEvent(ctx, msg.Event)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(msg.Event.JSON()),
			"pdupos":     pduPos,
			log.ErrorKey: err,
		}).Panicf("roomserver output log: write invite failure")
		return nil
	}
	s.notifier.OnNewEvent(&msg.Event, "", nil, types.NewStreamToken(pduPos, 0))
	return nil
}

func (s *OutputRoomEventConsumer) onRetireInviteEvent(
	ctx context.Context, msg api.OutputRetireInviteEvent,
) error {
	err := s.db.RetireInviteEvent(ctx, msg.EventID)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event_id":   msg.EventID,
			log.ErrorKey: err,
		}).Panicf("roomserver output log: remove invite failure")
		return nil
	}
	// TODO: Notify any active sync requests that the invite has been retired.
	// s.notifier.OnNewEvent(nil, msg.TargetUserID, syncStreamPos)
	return nil
}

func (s *OutputRoomEventConsumer) updateStateEvent(event gomatrixserverlib.HeaderedEvent) (gomatrixserverlib.HeaderedEvent, error) {
	var stateKey string
	if event.StateKey() == nil {
		stateKey = ""
	} else {
		stateKey = *event.StateKey()
	}

	prevEvent, err := s.db.GetStateEvent(
		context.TODO(), event.RoomID(), event.Type(), stateKey,
	)
	if err != nil {
		return event, err
	}

	if prevEvent == nil {
		return event, nil
	}

	prev := types.PrevEventRef{
		PrevContent:   prevEvent.Content(),
		ReplacesState: prevEvent.EventID(),
		PrevSender:    prevEvent.Sender(),
	}

	event.Event, err = event.SetUnsigned(prev)
	return event, err
}
