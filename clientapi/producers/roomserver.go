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
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomserverProducer produces events for the roomserver to consume.
type RoomserverProducer struct {
	InputAPI api.RoomserverInputAPI
	QueryAPI api.RoomserverQueryAPI
}

// NewRoomserverProducer creates a new RoomserverProducer
func NewRoomserverProducer(inputAPI api.RoomserverInputAPI, queryAPI api.RoomserverQueryAPI) *RoomserverProducer {
	return &RoomserverProducer{
		InputAPI: inputAPI,
		QueryAPI: queryAPI,
	}
}

// SendEvents writes the given events to the roomserver input log. The events are written with KindNew.
func (c *RoomserverProducer) SendEvents(
	ctx context.Context, events []gomatrixserverlib.HeaderedEvent, sendAsServer gomatrixserverlib.ServerName,
	txnID *api.TransactionID,
) (string, error) {
	ires := make([]api.InputRoomEvent, len(events))
	for i, event := range events {
		ires[i] = api.InputRoomEvent{
			Kind:          api.KindNew,
			Event:         event,
			AuthEventIDs:  event.AuthEventIDs(),
			SendAsServer:  string(sendAsServer),
			TransactionID: txnID,
		}
	}
	return c.SendInputRoomEvents(ctx, ires)
}

// SendEventWithState writes an event with KindNew to the roomserver input log
// with the state at the event as KindOutlier before it.
func (c *RoomserverProducer) SendEventWithState(
	ctx context.Context, state gomatrixserverlib.RespState, event gomatrixserverlib.HeaderedEvent,
) error {
	outliers, err := state.Events()
	if err != nil {
		return err
	}

	var ires []api.InputRoomEvent
	for _, outlier := range outliers {
		ires = append(ires, api.InputRoomEvent{
			Kind:         api.KindOutlier,
			Event:        outlier.Headered(event.RoomVersion),
			AuthEventIDs: outlier.AuthEventIDs(),
		})
	}

	stateEventIDs := make([]string, len(state.StateEvents))
	for i := range state.StateEvents {
		stateEventIDs[i] = state.StateEvents[i].EventID()
	}

	ires = append(ires, api.InputRoomEvent{
		Kind:          api.KindNew,
		Event:         event,
		AuthEventIDs:  event.AuthEventIDs(),
		HasState:      true,
		StateEventIDs: stateEventIDs,
	})

	_, err = c.SendInputRoomEvents(ctx, ires)
	return err
}

// SendInputRoomEvents writes the given input room events to the roomserver input API.
func (c *RoomserverProducer) SendInputRoomEvents(
	ctx context.Context, ires []api.InputRoomEvent,
) (eventID string, err error) {
	request := api.InputRoomEventsRequest{InputRoomEvents: ires}
	var response api.InputRoomEventsResponse
	err = c.InputAPI.InputRoomEvents(ctx, &request, &response)
	eventID = response.EventID
	return
}
