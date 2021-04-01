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
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// SendEvents to the roomserver The events are written with KindNew.
func SendEvents(
	ctx context.Context, rsAPI RoomserverInternalAPI,
	kind Kind, events []*gomatrixserverlib.HeaderedEvent,
	sendAsServer gomatrixserverlib.ServerName, txnID *TransactionID,
) error {
	ires := make([]InputRoomEvent, len(events))
	for i, event := range events {
		ires[i] = InputRoomEvent{
			Kind:          kind,
			Event:         event,
			AuthEventIDs:  event.AuthEventIDs(),
			SendAsServer:  string(sendAsServer),
			TransactionID: txnID,
		}
	}
	return SendInputRoomEvents(ctx, rsAPI, ires)
}

// SendEventWithState writes an event with the specified kind to the roomserver
// with the state at the event as KindOutlier before it. Will not send any event that is
// marked as `true` in haveEventIDs.
func SendEventWithState(
	ctx context.Context, rsAPI RoomserverInternalAPI, kind Kind,
	state *gomatrixserverlib.RespState, event *gomatrixserverlib.HeaderedEvent,
	haveEventIDs map[string]bool,
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
		Kind:          kind,
		Event:         event,
		AuthEventIDs:  event.AuthEventIDs(),
		HasState:      true,
		StateEventIDs: stateEventIDs,
	})

	return SendInputRoomEvents(ctx, rsAPI, ires)
}

// SendInputRoomEvents to the roomserver.
func SendInputRoomEvents(
	ctx context.Context, rsAPI RoomserverInternalAPI, ires []InputRoomEvent,
) error {
	request := InputRoomEventsRequest{InputRoomEvents: ires}
	var response InputRoomEventsResponse
	rsAPI.InputRoomEvents(ctx, &request, &response)
	return response.Err()
}

// SendInvite event to the roomserver.
// This should only be needed for invite events that occur outside of a known room.
// If we are in the room then the event should be sent using the SendEvents method.
func SendInvite(
	ctx context.Context,
	rsAPI RoomserverInternalAPI, inviteEvent *gomatrixserverlib.HeaderedEvent,
	inviteRoomState []gomatrixserverlib.InviteV2StrippedState,
	sendAsServer gomatrixserverlib.ServerName, txnID *TransactionID,
) error {
	// Start by sending the invite request into the roomserver. This will
	// trigger the federation request amongst other things if needed.
	request := &PerformInviteRequest{
		Event:           inviteEvent,
		InviteRoomState: inviteRoomState,
		RoomVersion:     inviteEvent.RoomVersion,
		SendAsServer:    string(sendAsServer),
		TransactionID:   txnID,
	}
	response := &PerformInviteResponse{}
	if err := rsAPI.PerformInvite(ctx, request, response); err != nil {
		return fmt.Errorf("rsAPI.PerformInvite: %w", err)
	}
	if response.Error != nil {
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
	return res.Events[0]
}

// GetStateEvent returns the current state event in the room or nil.
func GetStateEvent(ctx context.Context, rsAPI RoomserverInternalAPI, roomID string, tuple gomatrixserverlib.StateKeyTuple) *gomatrixserverlib.HeaderedEvent {
	var res QueryCurrentStateResponse
	err := rsAPI.QueryCurrentState(ctx, &QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{tuple},
	}, &res)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("Failed to QueryCurrentState")
		return nil
	}
	ev, ok := res.StateEvents[tuple]
	if ok {
		return ev
	}
	return nil
}

// IsServerBannedFromRoom returns whether the server is banned from a room by server ACLs.
func IsServerBannedFromRoom(ctx context.Context, rsAPI RoomserverInternalAPI, roomID string, serverName gomatrixserverlib.ServerName) bool {
	req := &QueryServerBannedFromRoomRequest{
		ServerName: serverName,
		RoomID:     roomID,
	}
	res := &QueryServerBannedFromRoomResponse{}
	if err := rsAPI.QueryServerBannedFromRoom(ctx, req, res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("Failed to QueryServerBannedFromRoom")
		return true
	}
	return res.Banned
}

// PopulatePublicRooms extracts PublicRoom information for all the provided room IDs. The IDs are not checked to see if they are visible in the
// published room directory.
// due to lots of switches
func PopulatePublicRooms(ctx context.Context, roomIDs []string, rsAPI RoomserverInternalAPI) ([]gomatrixserverlib.PublicRoom, error) {
	avatarTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.avatar", StateKey: ""}
	nameTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.name", StateKey: ""}
	canonicalTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomCanonicalAlias, StateKey: ""}
	topicTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.topic", StateKey: ""}
	guestTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.guest_access", StateKey: ""}
	visibilityTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomHistoryVisibility, StateKey: ""}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomJoinRules, StateKey: ""}

	var stateRes QueryBulkStateContentResponse
	err := rsAPI.QueryBulkStateContent(ctx, &QueryBulkStateContentRequest{
		RoomIDs:        roomIDs,
		AllowWildcards: true,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			nameTuple, canonicalTuple, topicTuple, guestTuple, visibilityTuple, joinRuleTuple, avatarTuple,
			{EventType: gomatrixserverlib.MRoomMember, StateKey: "*"},
		},
	}, &stateRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryBulkStateContent failed")
		return nil, err
	}
	chunk := make([]gomatrixserverlib.PublicRoom, len(roomIDs))
	i := 0
	for roomID, data := range stateRes.Rooms {
		pub := gomatrixserverlib.PublicRoom{
			RoomID: roomID,
		}
		joinCount := 0
		var joinRule, guestAccess string
		for tuple, contentVal := range data {
			if tuple.EventType == gomatrixserverlib.MRoomMember && contentVal == "join" {
				joinCount++
				continue
			}
			switch tuple {
			case avatarTuple:
				pub.AvatarURL = contentVal
			case nameTuple:
				pub.Name = contentVal
			case topicTuple:
				pub.Topic = contentVal
			case canonicalTuple:
				pub.CanonicalAlias = contentVal
			case visibilityTuple:
				pub.WorldReadable = contentVal == "world_readable"
			// need both of these to determine whether guests can join
			case joinRuleTuple:
				joinRule = contentVal
			case guestTuple:
				guestAccess = contentVal
			}
		}
		if joinRule == gomatrixserverlib.Public && guestAccess == "can_join" {
			pub.GuestCanJoin = true
		}
		pub.JoinedMembersCount = joinCount
		chunk[i] = pub
		i++
	}
	return chunk, nil
}
