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
}

// NewRoomserverProducer creates a new RoomserverProducer
func NewRoomserverProducer(inputAPI api.RoomserverInputAPI) *RoomserverProducer {
	return &RoomserverProducer{
		InputAPI: inputAPI,
	}
}

// SendEvents writes the given events to the roomserver input log. The events are written with KindNew.
func (c *RoomserverProducer) SendEvents(
	ctx context.Context, events []gomatrixserverlib.Event, sendAsServer gomatrixserverlib.ServerName,
	txnID *api.TransactionID,
) (string, error) {
	ires := make([]api.InputRoomEvent, len(events))
	for i, event := range events {
		roomVersion := gomatrixserverlib.RoomVersionV1

		ires[i] = api.InputRoomEvent{
			Kind:          api.KindNew,
			Event:         event.Headered(roomVersion),
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
	ctx context.Context, state gomatrixserverlib.RespState, event gomatrixserverlib.Event,
) error {
	outliers, err := state.Events()
	if err != nil {
		return err
	}

	// TODO: Room version here
	roomVersion := gomatrixserverlib.RoomVersionV1

	ires := make([]api.InputRoomEvent, len(outliers)+1)
	for i, outlier := range outliers {
		ires[i] = api.InputRoomEvent{
			Kind:         api.KindOutlier,
			Event:        outlier.Headered(roomVersion),
			AuthEventIDs: outlier.AuthEventIDs(),
		}
	}

	stateEventIDs := make([]string, len(state.StateEvents))
	for i := range state.StateEvents {
		stateEventIDs[i] = state.StateEvents[i].EventID()
	}

	ires[len(outliers)] = api.InputRoomEvent{
		Kind:          api.KindNew,
		Event:         event.Headered(roomVersion),
		AuthEventIDs:  event.AuthEventIDs(),
		HasState:      true,
		StateEventIDs: stateEventIDs,
	}

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

// SendInvite writes the invite event to the roomserver input API.
// This should only be needed for invite events that occur outside of a known room.
// If we are in the room then the event should be sent using the SendEvents method.
func (c *RoomserverProducer) SendInvite(
	ctx context.Context, inviteEvent gomatrixserverlib.Event,
) error {
	// TODO: Room version here
	roomVersion := gomatrixserverlib.RoomVersionV1

	request := api.InputRoomEventsRequest{
		InputInviteEvents: []api.InputInviteEvent{{
			Event: inviteEvent.Headered(roomVersion),
		}},
	}
	var response api.InputRoomEventsResponse
	return c.InputAPI.InputRoomEvents(ctx, &request, &response)
}
