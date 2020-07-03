// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/dendrite/currentstateserver/storage"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type OutputRoomEventConsumer struct {
	rsConsumer *internal.ContinualConsumer
	db         storage.Database
}

func NewOutputRoomEventConsumer(topicName string, kafkaConsumer sarama.Consumer, store storage.Database) *OutputRoomEventConsumer {
	consumer := &internal.ContinualConsumer{
		Topic:          topicName,
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEventConsumer{
		rsConsumer: consumer,
		db:         store,
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

func (c *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output api.OutputEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	switch output.Type {
	case api.OutputTypeNewRoomEvent:
		return c.onNewRoomEvent(context.TODO(), *output.NewRoomEvent)
	case api.OutputTypeNewInviteEvent:
	case api.OutputTypeRetireInviteEvent:
	default:
		log.WithField("type", output.Type).Debug(
			"roomserver output log: ignoring unknown output type",
		)
	}
	return nil
}

func (c *OutputRoomEventConsumer) onNewRoomEvent(
	ctx context.Context, msg api.OutputNewRoomEvent,
) error {
	ev := msg.Event

	addsStateEvents := msg.AddsState()

	ev, err := c.updateStateEvent(ev)
	if err != nil {
		return err
	}

	for i := range addsStateEvents {
		addsStateEvents[i], err = c.updateStateEvent(addsStateEvents[i])
		if err != nil {
			return err
		}
	}

	err = c.db.StoreStateEvents(
		ctx,
		addsStateEvents,
		msg.RemovesStateEventIDs,
	)
	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
			"add":        msg.AddsStateEventIDs,
			"del":        msg.RemovesStateEventIDs,
		}).Panicf("roomserver output log: write event failure")
	}
	return nil
}

// Start consuming from room servers
func (c *OutputRoomEventConsumer) Start() error {
	return c.rsConsumer.Start()
}

func (c *OutputRoomEventConsumer) updateStateEvent(event gomatrixserverlib.HeaderedEvent) (gomatrixserverlib.HeaderedEvent, error) {
	var stateKey string
	if event.StateKey() == nil {
		stateKey = ""
	} else {
		stateKey = *event.StateKey()
	}

	prevEvent, err := c.db.GetStateEvent(
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
