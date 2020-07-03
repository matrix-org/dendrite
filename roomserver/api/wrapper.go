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

package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// SendEvents to the roomserver The events are written with KindNew.
func SendEvents(
	ctx context.Context, rsAPI RoomserverInternalAPI, events []gomatrixserverlib.HeaderedEvent,
	sendAsServer gomatrixserverlib.ServerName, txnID *TransactionID,
) (string, error) {
	ires := make([]InputRoomEvent, len(events))
	for i, event := range events {
		ires[i] = InputRoomEvent{
			Kind:          KindNew,
			Event:         event,
			AuthEventIDs:  event.AuthEventIDs(),
			SendAsServer:  string(sendAsServer),
			TransactionID: txnID,
		}
	}
	return SendInputRoomEvents(ctx, rsAPI, ires)
}

// SendEventWithState writes an event with KindNew to the roomserver
// with the state at the event as KindOutlier before it. Will not send any event that is
// marked as `true` in haveEventIDs
func SendEventWithState(
	ctx context.Context, rsAPI RoomserverInternalAPI, state *gomatrixserverlib.RespState,
	event gomatrixserverlib.HeaderedEvent, haveEventIDs map[string]bool,
) error {
	outliers, err := state.Events()
	if err != nil {
		return err
	}

	var ires []InputRoomEvent
	for _, outlier := range outliers {
		if haveEventIDs[outlier.EventID()] {
			continue
		}
		ires = append(ires, InputRoomEvent{
			Kind:         KindOutlier,
			Event:        outlier.Headered(event.RoomVersion),
			AuthEventIDs: outlier.AuthEventIDs(),
		})
	}

	stateEventIDs := make([]string, len(state.StateEvents))
	for i := range state.StateEvents {
		stateEventIDs[i] = state.StateEvents[i].EventID()
	}

	ires = append(ires, InputRoomEvent{
		Kind:          KindNew,
		Event:         event,
		AuthEventIDs:  event.AuthEventIDs(),
		HasState:      true,
		StateEventIDs: stateEventIDs,
	})

	_, err = SendInputRoomEvents(ctx, rsAPI, ires)
	return err
}

// SendInputRoomEvents to the roomserver.
func SendInputRoomEvents(
	ctx context.Context, rsAPI RoomserverInternalAPI, ires []InputRoomEvent,
) (eventID string, err error) {
	request := InputRoomEventsRequest{InputRoomEvents: ires}
	var response InputRoomEventsResponse
	err = rsAPI.InputRoomEvents(ctx, &request, &response)
	eventID = response.EventID
	return
}

// SendInvite event to the roomserver.
// This should only be needed for invite events that occur outside of a known room.
// If we are in the room then the event should be sent using the SendEvents method.
func SendInvite(
	ctx context.Context,
	rsAPI RoomserverInternalAPI, inviteEvent gomatrixserverlib.HeaderedEvent,
	inviteRoomState []gomatrixserverlib.InviteV2StrippedState,
	sendAsServer gomatrixserverlib.ServerName, txnID *TransactionID,
) *PerformError {
	request := PerformInviteRequest{
		Event:           inviteEvent,
		InviteRoomState: inviteRoomState,
		RoomVersion:     inviteEvent.RoomVersion,
		SendAsServer:    string(sendAsServer),
		TransactionID:   txnID,
	}
	var response PerformInviteResponse
	rsAPI.PerformInvite(ctx, &request, &response)
	// we need to do this because many places people will use `var err error` as the return
	// arg and a nil interface != nil pointer to a concrete interface (in this case PerformError)
	if response.Error != nil && response.Error.Msg != "" {
		return response.Error
	}
	return nil
}

// GetEvent returns the event or nil, even on errors.
func GetEvent(ctx context.Context, rsAPI RoomserverInternalAPI, eventID string) *gomatrixserverlib.HeaderedEvent {
	var res QueryEventsByIDResponse
	err := rsAPI.QueryEventsByID(ctx, &QueryEventsByIDRequest{
		EventIDs: []string{eventID},
	}, &res)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("Failed to QueryEventsByID")
		return nil
	}
	if len(res.Events) != 1 {
		return nil
	}
	return &res.Events[0]
}
