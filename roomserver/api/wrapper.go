// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package api

import (
	"context"

	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// SendEvents to the roomserver The events are written with KindNew.
func SendEvents(
	ctx context.Context, rsAPI InputRoomEventsAPI,
	kind Kind, events []*types.HeaderedEvent,
	virtualHost, origin spec.ServerName,
	sendAsServer spec.ServerName, txnID *TransactionID,
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
	return SendInputRoomEvents(ctx, rsAPI, virtualHost, ires, async)
}

// SendEventWithState writes an event with the specified kind to the roomserver
// with the state at the event as KindOutlier before it. Will not send any event that is
// marked as `true` in haveEventIDs.
func SendEventWithState(
	ctx context.Context, rsAPI InputRoomEventsAPI,
	virtualHost spec.ServerName, kind Kind,
	state gomatrixserverlib.StateResponse, event *types.HeaderedEvent,
	origin spec.ServerName, haveEventIDs map[string]bool, async bool,
) error {
	outliers := gomatrixserverlib.LineariseStateResponse(event.Version(), state)
	ires := make([]InputRoomEvent, 0, len(outliers))
	for _, outlier := range outliers {
		if haveEventIDs[outlier.EventID()] {
			continue
		}
		ires = append(ires, InputRoomEvent{
			Kind:   KindOutlier,
			Event:  &types.HeaderedEvent{PDU: outlier},
			Origin: origin,
		})
	}

	stateEvents := state.GetStateEvents().UntrustedEvents(event.Version())
	stateEventIDs := make([]string, len(stateEvents))
	for i := range stateEvents {
		stateEventIDs[i] = stateEvents[i].EventID()
	}

	logrus.WithContext(ctx).WithFields(logrus.Fields{
		"room_id":   event.RoomID().String(),
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

	return SendInputRoomEvents(ctx, rsAPI, virtualHost, ires, async)
}

// SendInputRoomEvents to the roomserver.
func SendInputRoomEvents(
	ctx context.Context, rsAPI InputRoomEventsAPI,
	virtualHost spec.ServerName,
	ires []InputRoomEvent, async bool,
) error {
	request := InputRoomEventsRequest{
		InputRoomEvents: ires,
		Asynchronous:    async,
		VirtualHost:     virtualHost,
	}
	var response InputRoomEventsResponse
	rsAPI.InputRoomEvents(ctx, &request, &response)
	return response.Err()
}

// GetEvent returns the event or nil, even on errors.
func GetEvent(ctx context.Context, rsAPI QueryEventsAPI, roomID, eventID string) *types.HeaderedEvent {
	var res QueryEventsByIDResponse
	err := rsAPI.QueryEventsByID(ctx, &QueryEventsByIDRequest{
		RoomID:   roomID,
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
func GetStateEvent(ctx context.Context, rsAPI QueryEventsAPI, roomID string, tuple gomatrixserverlib.StateKeyTuple) *types.HeaderedEvent {
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
func IsServerBannedFromRoom(ctx context.Context, rsAPI FederationRoomserverAPI, roomID string, serverName spec.ServerName) bool {
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
func PopulatePublicRooms(ctx context.Context, roomIDs []string, rsAPI QueryBulkStateContentAPI) ([]fclient.PublicRoom, error) {
	avatarTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.avatar", StateKey: ""}
	nameTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.name", StateKey: ""}
	canonicalTuple := gomatrixserverlib.StateKeyTuple{EventType: spec.MRoomCanonicalAlias, StateKey: ""}
	topicTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.topic", StateKey: ""}
	guestTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.guest_access", StateKey: ""}
	visibilityTuple := gomatrixserverlib.StateKeyTuple{EventType: spec.MRoomHistoryVisibility, StateKey: ""}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{EventType: spec.MRoomJoinRules, StateKey: ""}

	var stateRes QueryBulkStateContentResponse
	err := rsAPI.QueryBulkStateContent(ctx, &QueryBulkStateContentRequest{
		RoomIDs:        roomIDs,
		AllowWildcards: true,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			nameTuple, canonicalTuple, topicTuple, guestTuple, visibilityTuple, joinRuleTuple, avatarTuple,
			{EventType: spec.MRoomMember, StateKey: "*"},
		},
	}, &stateRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryBulkStateContent failed")
		return nil, err
	}
	chunk := make([]fclient.PublicRoom, len(roomIDs))
	i := 0
	for roomID, data := range stateRes.Rooms {
		pub := fclient.PublicRoom{
			RoomID: roomID,
		}
		joinCount := 0
		var guestAccess string
		for tuple, contentVal := range data {
			if tuple.EventType == spec.MRoomMember && contentVal == "join" {
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
				pub.JoinRule = contentVal
			case guestTuple:
				guestAccess = contentVal
			}
		}
		if pub.JoinRule == spec.Public && guestAccess == "can_join" {
			pub.GuestCanJoin = true
		}
		pub.JoinedMembersCount = joinCount
		chunk[i] = pub
		i++
	}
	return chunk, nil
}
