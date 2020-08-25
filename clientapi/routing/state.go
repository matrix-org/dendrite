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

package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

// OnIncomingStateTypeRequest is called when a client makes a request to either:
// - /rooms/{roomID}/state
// - /rooms/{roomID}/state/{type}/{statekey}
// The former will have evType and stateKey as "". The latter will specify them.
// It will look in current state to see if there is an event with that type
// and state key, if there is then (by default) we return the content. If the
// history visibility restricts the room state then users who have left or
// have been kicked/banned from the room will see the state from the time that
// they left.
// nolint:gocyclo
func OnIncomingStateTypeRequest(
	ctx context.Context, device *userapi.Device, rsAPI api.RoomserverInternalAPI,
	roomID, evType, stateKey string,
) util.JSONResponse {
	var worldReadable bool
	var wantLatestState bool
	wantAllState := evType == ""

	// Work out the state to fetch. If we want the entire room state then
	// we leave the stateToFetch empty. Otherwise we specify what we want.
	stateToFetch := []gomatrixserverlib.StateKeyTuple{}
	if !wantAllState {
		stateToFetch = append(stateToFetch, gomatrixserverlib.StateKeyTuple{
			EventType: evType,
			StateKey:  stateKey,
		})
	}

	// Always fetch visibility so that we can work out whether to show
	// the latest events or the last event from when the user was joined.
	// Then include the requested event type and state key, assuming it
	// isn't for the same.
	stateToFetchIncVisibility := stateToFetch
	if !wantAllState && evType != gomatrixserverlib.MRoomHistoryVisibility && stateKey != "" {
		stateToFetchIncVisibility = append(
			stateToFetchIncVisibility,
			gomatrixserverlib.StateKeyTuple{
				EventType: gomatrixserverlib.MRoomHistoryVisibility,
				StateKey:  "",
			},
		)
	}

	// First of all, get the latest state of the room. We need to do this
	// so that we can look at the history visibility of the room. If the
	// room is world-readable then we will always return the latest state.
	stateRes := api.QueryLatestEventsAndStateResponse{}
	if err := rsAPI.QueryLatestEventsAndState(ctx, &api.QueryLatestEventsAndStateRequest{
		RoomID:       roomID,
		StateToFetch: stateToFetchIncVisibility,
	}, &stateRes); err != nil {
		util.GetLogger(ctx).WithError(err).Error("queryAPI.QueryLatestEventsAndState failed")
		return jsonerror.InternalServerError()
	}

	// Look at the room state and see if we have a history visibility event
	// that marks the room as world-readable. If we don't then we assume that
	// the room is not world-readable.
	for _, ev := range stateRes.StateEvents {
		if ev.Type() == gomatrixserverlib.MRoomHistoryVisibility {
			content := map[string]string{}
			if err := json.Unmarshal(ev.Content(), &content); err != nil {
				util.GetLogger(ctx).WithError(err).Error("json.Unmarshal for history visibility failed")
				return jsonerror.InternalServerError()
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
		err := rsAPI.QueryMembershipForUser(ctx, &api.QueryMembershipForUserRequest{
			RoomID: roomID,
			UserID: device.UserID,
		}, &membershipRes)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("Failed to QueryMembershipForUser")
			return jsonerror.InternalServerError()
		}
		// If the user has never been in the room then stop at this point.
		// We won't tell the user about a room they have never joined.
		if !membershipRes.HasBeenInRoom {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden(fmt.Sprintf("Unknown room %q or user %q has never joined this room", roomID, device.UserID)),
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

	var events []*gomatrixserverlib.HeaderedEvent
	if wantLatestState {
		// If we are happy to use the latest state, either because the user is
		// still in the room, or because the room is world-readable, then just
		// use the result of the previous QueryLatestEventsAndState response
		// to find the state event, if provided.
		for _, ev := range stateRes.StateEvents {
			if ev.Type() == evType && ev.StateKeyEquals(stateKey) {
				events = append(events, &ev)
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
			StateToFetch: stateToFetch,
		}, &stateAfterRes)
		if err != nil {
			util.GetLogger(ctx).WithError(err).Error("Failed to QueryMembershipForUser")
			return jsonerror.InternalServerError()
		}
		if len(stateAfterRes.StateEvents) > 0 {
			events = append(events, &stateAfterRes.StateEvents[0])
		}
	}

	// If there was no event found that matches all of the above criteria then
	// return an error.
	if !wantAllState && len(events) == 0 {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(fmt.Sprintf("Cannot find state event for %q", evType)),
		}
	}

	// Turn the events into client events.
	var res []gomatrixserverlib.ClientEvent
	for _, event := range events {
		res = append(
			res,
			gomatrixserverlib.HeaderedToClientEvent(*event, gomatrixserverlib.FormatAll),
		)
	}

	// Return the client events.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}
