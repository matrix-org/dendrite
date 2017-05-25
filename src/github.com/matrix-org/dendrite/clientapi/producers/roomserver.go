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

package producers

import (
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// RoomserverProducer produces events for the roomserver to consume.
type RoomserverProducer struct {
	Topic    string
	Producer sarama.SyncProducer
}

// NewRoomserverProducer creates a new RoomserverProducer
func NewRoomserverProducer(kafkaURIs []string, topic string) (*RoomserverProducer, error) {
	producer, err := sarama.NewSyncProducer(kafkaURIs, nil)
	if err != nil {
		return nil, err
	}
	return &RoomserverProducer{
		Topic:    topic,
		Producer: producer,
	}, nil
}

// SendEvents writes the given events to the roomserver input log. The events are written with KindNew.
func (c *RoomserverProducer) SendEvents(events []gomatrixserverlib.Event) error {
	eventIDs := make([]string, len(events))
	ires := make([]api.InputRoomEvent, len(events))
	for i, event := range events {
		ires[i] = api.InputRoomEvent{
			Kind:         api.KindNew,
			Event:        event.JSON(),
			AuthEventIDs: authEventIDs(event),
		}
		eventIDs[i] = event.EventID()
	}
	return c.SendInputRoomEvents(ires, eventIDs)
}

// SendEventWithState writes an event with KindNew to the roomserver input log
// with the state at the event as KindOutlier before it.
func (c *RoomserverProducer) SendEventWithState(state gomatrixserverlib.RespState, event gomatrixserverlib.Event) error {
	outliers, err := state.Events()
	if err != nil {
		return err
	}

	eventIDs := make([]string, len(outliers)+1)
	ires := make([]api.InputRoomEvent, len(outliers)+1)
	for i, outlier := range outliers {
		ires[i] = api.InputRoomEvent{
			Kind:         api.KindOutlier,
			Event:        outlier.JSON(),
			AuthEventIDs: authEventIDs(outlier),
		}
		eventIDs[i] = outlier.EventID()
	}

	stateEventIDs := make([]string, len(state.StateEvents))
	for i := range state.StateEvents {
		stateEventIDs[i] = state.StateEvents[i].EventID()
	}

	ires[len(outliers)] = api.InputRoomEvent{
		Kind:          api.KindNew,
		Event:         event.JSON(),
		AuthEventIDs:  authEventIDs(event),
		HasState:      true,
		StateEventIDs: stateEventIDs,
	}
	eventIDs[len(outliers)] = event.EventID()

	return c.SendInputRoomEvents(ires, eventIDs)
}

// TODO Make this a method on gomatrixserverlib.Event
func authEventIDs(event gomatrixserverlib.Event) (ids []string) {
	for _, ref := range event.AuthEvents() {
		ids = append(ids, ref.EventID)
	}
	return
}

// SendInputRoomEvents writes the given input room events to the roomserver input log. The length of both
// arrays must match, and each element must correspond to the same event.
func (c *RoomserverProducer) SendInputRoomEvents(ires []api.InputRoomEvent, eventIDs []string) error {
	// TODO: Nicer way of doing this. Options are:
	// A) Like this
	// B) Add EventID field to InputRoomEvent
	// C) Add wrapper struct with the EventID and the InputRoomEvent
	if len(eventIDs) != len(ires) {
		return fmt.Errorf("WriteInputRoomEvents: length mismatch %d != %d", len(eventIDs), len(ires))
	}

	msgs := make([]*sarama.ProducerMessage, len(ires))
	for i := range ires {
		msg, err := c.toProducerMessage(ires[i], eventIDs[i])
		if err != nil {
			return err
		}
		msgs[i] = msg
	}
	return c.Producer.SendMessages(msgs)
}

func (c *RoomserverProducer) toProducerMessage(ire api.InputRoomEvent, eventID string) (*sarama.ProducerMessage, error) {
	value, err := json.Marshal(ire)
	if err != nil {
		return nil, err
	}
	var m sarama.ProducerMessage
	m.Topic = c.Topic
	m.Key = sarama.StringEncoder(eventID)
	m.Value = sarama.ByteEncoder(value)
	return &m, nil
}
