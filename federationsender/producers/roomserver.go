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

package producers

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomserverProducer produces events for the roomserver to consume.
type RoomserverProducer struct {
	InputAPI   api.RoomserverInputAPI
	serverName gomatrixserverlib.ServerName
}

// NewRoomserverProducer creates a new RoomserverProducer
func NewRoomserverProducer(
	inputAPI api.RoomserverInputAPI, serverName gomatrixserverlib.ServerName,
) *RoomserverProducer {
	return &RoomserverProducer{
		InputAPI:   inputAPI,
		serverName: serverName,
	}
}

// SendInviteResponse drops an invite response back into the roomserver so that users
// already in the room will be notified of the new invite. The invite response is signed
// by the remote side.
func (c *RoomserverProducer) SendInviteResponse(
	ctx context.Context, res gomatrixserverlib.RespInviteV2, roomVersion gomatrixserverlib.RoomVersion,
) (string, error) {
	ev := res.Event.Headered(roomVersion)
	ire := api.InputRoomEvent{
		Kind:          api.KindNew,
		Event:         ev,
		AuthEventIDs:  ev.AuthEventIDs(),
		SendAsServer:  string(c.serverName),
		TransactionID: nil,
	}
	return c.SendInputRoomEvents(ctx, []api.InputRoomEvent{ire})
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
