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
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomserverProducer produces events for the roomserver to consume.
type RoomserverProducer struct {
	InputAPI api.RoomserverInputAPI
}

// NewRoomserverProducer creates a new RoomserverProducer
func NewRoomserverProducer(inputAPI api.RoomserverInputAPI) *RoomserverProducer {
	return &RoomserverProducer{
		InputAPI: inputAPI,
	}
}

// SendEvents writes the given events to the roomserver input log. The events are written with KindNew.
func (c *RoomserverProducer) SendEvents(events []gomatrixserverlib.Event, sendAsServer gomatrixserverlib.ServerName) error {
	ires := make([]api.InputRoomEvent, len(events))
	for i, event := range events {
		ires[i] = api.InputRoomEvent{
			Kind:         api.KindNew,
			Event:        event,
			AuthEventIDs: event.AuthEventIDs(),
			SendAsServer: string(sendAsServer),
		}
	}
	return c.SendInputRoomEvents(ires)
}

// SendEventWithState writes an event with KindNew to the roomserver input log
// with the state at the event as KindOutlier before it.
func (c *RoomserverProducer) SendEventWithState(state gomatrixserverlib.RespState, event gomatrixserverlib.Event) error {
	outliers, err := state.Events()
	if err != nil {
		return err
	}

	ires := make([]api.InputRoomEvent, len(outliers)+1)
	for i, outlier := range outliers {
		ires[i] = api.InputRoomEvent{
			Kind:         api.KindOutlier,
			Event:        outlier,
			AuthEventIDs: outlier.AuthEventIDs(),
		}
	}

	stateEventIDs := make([]string, len(state.StateEvents))
	for i := range state.StateEvents {
		stateEventIDs[i] = state.StateEvents[i].EventID()
	}

	ires[len(outliers)] = api.InputRoomEvent{
		Kind:          api.KindNew,
		Event:         event,
		AuthEventIDs:  event.AuthEventIDs(),
		HasState:      true,
		StateEventIDs: stateEventIDs,
	}

	return c.SendInputRoomEvents(ires)
}

// SendInputRoomEvents writes the given input room events to the roomserver input log. The length of both
// arrays must match, and each element must correspond to the same event.
func (c *RoomserverProducer) SendInputRoomEvents(ires []api.InputRoomEvent) error {
	request := api.InputRoomEventsRequest{InputRoomEvents: ires}
	var response api.InputRoomEventsResponse
	return c.InputAPI.InputRoomEvents(&request, &response)
}
