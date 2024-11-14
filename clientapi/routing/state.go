// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

type stateEventInStateResp struct {
	synctypes.ClientEvent
	PrevContent   json.RawMessage `json:"prev_content,omitempty"`
	ReplacesState string          `json:"replaces_state,omitempty"`
}

// OnIncomingStateRequest is called when a client makes a /rooms/{roomID}/state
// request. It will fetch all the state events from the specified room and will
// append the necessary keys to them if applicable before returning them.
// Returns an error if something went wrong in the process.
// TODO: Check if the user is in the room. If not, check if the room's history
// is publicly visible. Current behaviour is returning an empty array if the
// user cannot see the room's history.
func OnIncomingStateRequest(ctx context.Context, device *userapi.Device, rsAPI api.ClientRoomserverAPI, roomID string) util.JSONResponse {
	var worldReadable bool
	var wantLatestState bool

	// First of all, get the latest state of the room. We need to do this
	// so that we can look at the history visibility of the room. If the
	// room is world-readable then we will always return the latest state.
	stateRes := api.QueryLatestEventsAndStateResponse{}
	if err := rsAPI.QueryLatestEventsAndState(ctx, &api.QueryLatestEventsAndStateRequest{
		RoomID:       roomID,
		StateToFetch: []gomatrixserverlib.StateKeyTuple{},
	}, &stateRes); err != nil {
		util.GetLogger(ctx).WithError(err).Error("queryAPI.QueryLatestEventsAndState failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if !stateRes.RoomExists {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("room does not exist"),
		}
	}

	// Look at the room state and see if we have a history visibility event
	// that marks the room as world-readable. If we don't then we assume that
	// the room is not world-readable.
	for _, ev := range stateRes.StateEvents {
		if ev.Type() == spec.MRoomHistoryVisibility {
			content := map[string]string{}
			if err := json.Unmarshal(ev.Content(), &content); err != nil {
				util.GetLogger(ctx).WithError(err).Error("json.Unmarshal for history visibility failed")
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
			if visibility, ok := content["history_visibility"]; ok {
				worldReadable = visibility == "world_readable"
				break
			}
		}
	}

	// If the room isn't world-readable then we will instead try to find out
	// the state of the room based on the user's membership. If the user is
	// in the room then we'll want the latest state. If the user has never
	// been in the room and the room isn't world-readable, then we won't
	// return any state. If the user was in the room previously but is no
	// longer then we will return the state at the time that the user left.
	// membershipRes will only be populated if the room is not world-readable.
	var membershipRes api.QueryMembershipForUserResponse
	if !worldReadable {
		// The room isn't world-readable so try to work out based on the
		// user's membership if we want the latest state or not.
		userID, err := spec.NewUserID(device.UserID, true)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("UserID is invalid")
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.Unknown("Device UserID is invalid"),
			}
		}
		err = rsAPI.QueryMembershipForUser(ctx, &api.QueryMembershipForUserRequest{
			RoomID: roomID,
			UserID: *userID,
		}, &membershipRes)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("Failed to QueryMembershipForUser")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		// If the user has never been in the room then stop at this point.
		// We won't tell the user about a room they have never joined.
		if !membershipRes.HasBeenInRoom {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden(fmt.Sprintf("Unknown room %q or user %q has never joined this room", roomID, device.UserID)),
			}
		}
		// Otherwise, if the user has been in the room, whether or not we
		// give them the latest state will depend on if they are *still* in
		// the room.
		wantLatestState = membershipRes.IsInRoom
	} else {
		// The room is world-readable so the user join state is irrelevant,
		// just get the latest room state instead.
		wantLatestState = true
	}

	util.GetLogger(ctx).WithFields(log.Fields{
		"roomID":         roomID,
		"state_at_event": !wantLatestState,
	}).Info("Fetching all state")

	stateEvents := []synctypes.ClientEvent{}
	if wantLatestState {
		// If we are happy to use the latest state, either because the user is
		// still in the room, or because the room is world-readable, then just
		// use the result of the previous QueryLatestEventsAndState response
		// to find the state event, if provided.
		for _, ev := range stateRes.StateEvents {
			stateEvents = append(
				stateEvents,
				synctypes.ToClientEventDefault(func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
					return rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
				}, ev),
			)
		}
	} else {
		// Otherwise, take the event ID of their leave event and work out what
		// the state of the room was before that event.
		var stateAfterRes api.QueryStateAfterEventsResponse
		err := rsAPI.QueryStateAfterEvents(ctx, &api.QueryStateAfterEventsRequest{
			RoomID:       roomID,
			PrevEventIDs: []string{membershipRes.EventID},
			StateToFetch: []gomatrixserverlib.StateKeyTuple{},
		}, &stateAfterRes)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("Failed to QueryMembershipForUser")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		for _, ev := range stateAfterRes.StateEvents {
			clientEvent, err := synctypes.ToClientEvent(ev, synctypes.FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
				return rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
			})
			if err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed converting to ClientEvent")
				continue
			}
			stateEvents = append(
				stateEvents,
				*clientEvent,
			)
		}
	}

	// Return the results to the requestor.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: stateEvents,
	}
}

// OnIncomingStateTypeRequest is called when a client makes a
// /rooms/{roomID}/state/{type}/{statekey} request. It will look in current
// state to see if there is an event with that type and state key, if there
// is then (by default) we return the content, otherwise a 404.
// If eventFormat=true, sends the whole event else just the content.
func OnIncomingStateTypeRequest(
	ctx context.Context, device *userapi.Device, rsAPI api.ClientRoomserverAPI,
	roomID, evType, stateKey string, eventFormat bool,
) util.JSONResponse {
	var worldReadable bool
	var wantLatestState bool

	roomVer, err := rsAPI.QueryRoomVersionForRoom(ctx, roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden(fmt.Sprintf("Unknown room %q or user %q has never joined this room", roomID, device.UserID)),
		}
	}

	// Translate user ID state keys to room keys in pseudo ID rooms
	if roomVer == gomatrixserverlib.RoomVersionPseudoIDs {
		parsedRoomID, err := spec.NewRoomID(roomID)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: spec.InvalidParam("invalid room ID"),
			}
		}
		newStateKey, err := synctypes.FromClientStateKey(*parsedRoomID, stateKey, func(roomID spec.RoomID, userID spec.UserID) (*spec.SenderID, error) {
			return rsAPI.QuerySenderIDForUser(ctx, roomID, userID)
		})
		if err != nil {
			// TODO: work out better logic for failure cases (e.g. sender ID not found)
			util.GetLogger(ctx).WithError(err).Error("synctypes.FromClientStateKey failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown("internal server error"),
			}
		}
		stateKey = *newStateKey
	}

	// Always fetch visibility so that we can work out whether to show
	// the latest events or the last event from when the user was joined.
	// Then include the requested event type and state key, assuming it
	// isn't for the same.
	stateToFetch := []gomatrixserverlib.StateKeyTuple{
		{
			EventType: evType,
			StateKey:  stateKey,
		},
	}
	if evType != spec.MRoomHistoryVisibility && stateKey != "" {
		stateToFetch = append(stateToFetch, gomatrixserverlib.StateKeyTuple{
			EventType: spec.MRoomHistoryVisibility,
			StateKey:  "",
		})
	}

	// First of all, get the latest state of the room. We need to do this
	// so that we can look at the history visibility of the room. If the
	// room is world-readable then we will always return the latest state.
	stateRes := api.QueryLatestEventsAndStateResponse{}
	if err := rsAPI.QueryLatestEventsAndState(ctx, &api.QueryLatestEventsAndStateRequest{
		RoomID:       roomID,
		StateToFetch: stateToFetch,
	}, &stateRes); err != nil {
		util.GetLogger(ctx).WithError(err).Error("queryAPI.QueryLatestEventsAndState failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Look at the room state and see if we have a history visibility event
	// that marks the room as world-readable. If we don't then we assume that
	// the room is not world-readable.
	for _, ev := range stateRes.StateEvents {
		if ev.Type() == spec.MRoomHistoryVisibility {
			content := map[string]string{}
			if err := json.Unmarshal(ev.Content(), &content); err != nil {
				util.GetLogger(ctx).WithError(err).Error("json.Unmarshal for history visibility failed")
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
			if visibility, ok := content["history_visibility"]; ok {
				worldReadable = visibility == "world_readable"
				break
			}
		}
	}

	// If the room isn't world-readable then we will instead try to find out
	// the state of the room based on the user's membership. If the user is
	// in the room then we'll want the latest state. If the user has never
	// been in the room and the room isn't world-readable, then we won't
	// return any state. If the user was in the room previously but is no
	// longer then we will return the state at the time that the user left.
	// membershipRes will only be populated if the room is not world-readable.
	var membershipRes api.QueryMembershipForUserResponse
	if !worldReadable {
		userID, err := spec.NewUserID(device.UserID, true)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("UserID is invalid")
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.Unknown("Device UserID is invalid"),
			}
		}
		// The room isn't world-readable so try to work out based on the
		// user's membership if we want the latest state or not.
		err = rsAPI.QueryMembershipForUser(ctx, &api.QueryMembershipForUserRequest{
			RoomID: roomID,
			UserID: *userID,
		}, &membershipRes)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("Failed to QueryMembershipForUser")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		// If the user has never been in the room then stop at this point.
		// We won't tell the user about a room they have never joined.
		if !membershipRes.HasBeenInRoom || membershipRes.Membership == spec.Ban {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden(fmt.Sprintf("Unknown room %q or user %q has never joined this room", roomID, device.UserID)),
			}
		}
		// Otherwise, if the user has been in the room, whether or not we
		// give them the latest state will depend on if they are *still* in
		// the room.
		wantLatestState = membershipRes.IsInRoom
	} else {
		// The room is world-readable so the user join state is irrelevant,
		// just get the latest room state instead.
		wantLatestState = true
	}

	util.GetLogger(ctx).WithFields(log.Fields{
		"roomID":         roomID,
		"evType":         evType,
		"stateKey":       stateKey,
		"state_at_event": !wantLatestState,
	}).Info("Fetching state")

	var event *types.HeaderedEvent
	if wantLatestState {
		// If we are happy to use the latest state, either because the user is
		// still in the room, or because the room is world-readable, then just
		// use the result of the previous QueryLatestEventsAndState response
		// to find the state event, if provided.
		for _, ev := range stateRes.StateEvents {
			if ev.Type() == evType && ev.StateKeyEquals(stateKey) {
				event = ev
				break
			}
		}
	} else {
		// Otherwise, take the event ID of their leave event and work out what
		// the state of the room was before that event.
		var stateAfterRes api.QueryStateAfterEventsResponse
		err := rsAPI.QueryStateAfterEvents(ctx, &api.QueryStateAfterEventsRequest{
			RoomID:       roomID,
			PrevEventIDs: []string{membershipRes.EventID},
			StateToFetch: []gomatrixserverlib.StateKeyTuple{
				{
					EventType: evType,
					StateKey:  stateKey,
				},
			},
		}, &stateAfterRes)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("Failed to QueryMembershipForUser")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		if len(stateAfterRes.StateEvents) > 0 {
			event = stateAfterRes.StateEvents[0]
		}
	}

	// If there was no event found that matches all of the above criteria then
	// return an error.
	if event == nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound(fmt.Sprintf("Cannot find state event for %q", evType)),
		}
	}

	stateEvent := stateEventInStateResp{
		ClientEvent: synctypes.ToClientEventDefault(func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(ctx, roomID, senderID)
		}, event),
	}

	var res interface{}
	if eventFormat {
		res = stateEvent
	} else {
		res = stateEvent.Content
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}
