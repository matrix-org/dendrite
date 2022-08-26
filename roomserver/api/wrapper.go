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
	"github.com/sirupsen/logrus"
)

// SendEvents to the roomserver The events are written with KindNew.
func SendEvents(
	ctx context.Context, rsAPI InputRoomEventsAPI,
	kind Kind, events []*gomatrixserverlib.HeaderedEvent,
	origin gomatrixserverlib.ServerName,
	sendAsServer gomatrixserverlib.ServerName, txnID *TransactionID,
	async bool,
) error {
	ires := make([]InputRoomEvent, len(events))
	for i, event := range events {
		ires[i] = InputRoomEvent{
			Kind:          kind,
			Event:         event,
			Origin:        origin,
			SendAsServer:  string(sendAsServer),
			TransactionID: txnID,
		}
	}
	return SendInputRoomEvents(ctx, rsAPI, ires, async)
}

// SendEventWithState writes an event with the specified kind to the roomserver
// with the state at the event as KindOutlier before it. Will not send any event that is
// marked as `true` in haveEventIDs.
func SendEventWithState(
	ctx context.Context, rsAPI InputRoomEventsAPI, kind Kind,
	state *gomatrixserverlib.RespState, event *gomatrixserverlib.HeaderedEvent,
	origin gomatrixserverlib.ServerName, haveEventIDs map[string]bool, async bool,
) error {
	outliers := state.Events(event.RoomVersion)
	ires := make([]InputRoomEvent, 0, len(outliers))
	for _, outlier := range outliers {
		if haveEventIDs[outlier.EventID()] {
			continue
		}
		ires = append(ires, InputRoomEvent{
			Kind:   KindOutlier,
			Event:  outlier.Headered(event.RoomVersion),
			Origin: origin,
		})
	}

	stateEvents := state.StateEvents.UntrustedEvents(event.RoomVersion)
	stateEventIDs := make([]string, len(stateEvents))
	for i := range stateEvents {
		stateEventIDs[i] = stateEvents[i].EventID()
	}

	logrus.WithContext(ctx).WithFields(logrus.Fields{
		"room_id":   event.RoomID(),
		"event_id":  event.EventID(),
		"outliers":  len(ires),
		"state_ids": len(stateEventIDs),
	}).Infof("Submitting %q event to roomserver with state snapshot", event.Type())

	ires = append(ires, InputRoomEvent{
		Kind:          kind,
		Event:         event,
		Origin:        origin,
		HasState:      true,
		StateEventIDs: stateEventIDs,
	})

	return SendInputRoomEvents(ctx, rsAPI, ires, async)
}

// SendInputRoomEvents to the roomserver.
func SendInputRoomEvents(
	ctx context.Context, rsAPI InputRoomEventsAPI,
	ires []InputRoomEvent, async bool,
) error {
	request := InputRoomEventsRequest{
		InputRoomEvents: ires,
		Asynchronous:    async,
	}
	var response InputRoomEventsResponse
	if err := rsAPI.InputRoomEvents(ctx, &request, &response); err != nil {
		return err
	}
	return response.Err()
}

// GetEvent returns the event or nil, even on errors.
func GetEvent(ctx context.Context, rsAPI QueryEventsAPI, eventID string) *gomatrixserverlib.HeaderedEvent {
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
func GetStateEvent(ctx context.Context, rsAPI QueryEventsAPI, roomID string, tuple gomatrixserverlib.StateKeyTuple) *gomatrixserverlib.HeaderedEvent {
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
func IsServerBannedFromRoom(ctx context.Context, rsAPI FederationRoomserverAPI, roomID string, serverName gomatrixserverlib.ServerName) bool {
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
func PopulatePublicRooms(ctx context.Context, roomIDs []string, rsAPI QueryBulkStateContentAPI) ([]gomatrixserverlib.PublicRoom, error) {
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
				if _, _, err := gomatrixserverlib.SplitID('#', contentVal); err == nil {
					pub.CanonicalAlias = contentVal
				}
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
