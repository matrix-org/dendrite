// Copyright 2017 Vector Creations Ltd
// Copyright 2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package internal

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// QueryLatestEventsAndState implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryLatestEventsAndState(
	ctx context.Context,
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	roomInfo, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if roomInfo == nil || roomInfo.IsStub {
		response.RoomExists = false
		return nil
	}

	roomState := state.NewStateResolution(r.DB, *roomInfo)
	response.RoomExists = true
	response.RoomVersion = roomInfo.RoomVersion

	var currentStateSnapshotNID types.StateSnapshotNID
	response.LatestEvents, currentStateSnapshotNID, response.Depth, err =
		r.DB.LatestEventIDs(ctx, roomInfo.RoomNID)
	if err != nil {
		return err
	}

	var stateEntries []types.StateEntry
	if len(request.StateToFetch) == 0 {
		// Look up all room state.
		stateEntries, err = roomState.LoadStateAtSnapshot(
			ctx, currentStateSnapshotNID,
		)
	} else {
		// Look up the current state for the requested tuples.
		stateEntries, err = roomState.LoadStateAtSnapshotForStringTuples(
			ctx, currentStateSnapshotNID, request.StateToFetch,
		)
	}
	if err != nil {
		return err
	}

	stateEvents, err := helpers.LoadStateEvents(ctx, r.DB, stateEntries)
	if err != nil {
		return err
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, event.Headered(roomInfo.RoomVersion))
	}

	return nil
}

// QueryStateAfterEvents implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub {
		return nil
	}

	roomState := state.NewStateResolution(r.DB, *info)
	response.RoomExists = true
	response.RoomVersion = info.RoomVersion

	prevStates, err := r.DB.StateAtEventIDs(ctx, request.PrevEventIDs)
	if err != nil {
		switch err.(type) {
		case types.MissingEventError:
			return nil
		default:
			return err
		}
	}
	response.PrevEventsExist = true

	// Look up the currrent state for the requested tuples.
	stateEntries, err := roomState.LoadStateAfterEventsForStringTuples(
		ctx, prevStates, request.StateToFetch,
	)
	if err != nil {
		return err
	}

	stateEvents, err := helpers.LoadStateEvents(ctx, r.DB, stateEntries)
	if err != nil {
		return err
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, event.Headered(info.RoomVersion))
	}

	return nil
}

// QueryEventsByID implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryEventsByID(
	ctx context.Context,
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	eventNIDMap, err := r.DB.EventNIDs(ctx, request.EventIDs)
	if err != nil {
		return err
	}

	var eventNIDs []types.EventNID
	for _, nid := range eventNIDMap {
		eventNIDs = append(eventNIDs, nid)
	}

	events, err := helpers.LoadEvents(ctx, r.DB, eventNIDs)
	if err != nil {
		return err
	}

	for _, event := range events {
		roomVersion, verr := r.roomVersion(event.RoomID())
		if verr != nil {
			return verr
		}

		response.Events = append(response.Events, event.Headered(roomVersion))
	}

	return nil
}

// QueryMembershipForUser implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryMembershipForUser(
	ctx context.Context,
	request *api.QueryMembershipForUserRequest,
	response *api.QueryMembershipForUserResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}

	membershipEventNID, stillInRoom, err := r.DB.GetMembership(ctx, info.RoomNID, request.UserID)
	if err != nil {
		return err
	}

	if membershipEventNID == 0 {
		response.HasBeenInRoom = false
		return nil
	}

	response.IsInRoom = stillInRoom
	response.HasBeenInRoom = true

	evs, err := r.DB.Events(ctx, []types.EventNID{membershipEventNID})
	if err != nil {
		return err
	}
	if len(evs) != 1 {
		return fmt.Errorf("failed to load membership event for event NID %d", membershipEventNID)
	}

	response.EventID = evs[0].EventID()
	response.Membership, err = evs[0].Membership()
	return err
}

// QueryMembershipsForRoom implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryMembershipsForRoom(
	ctx context.Context,
	request *api.QueryMembershipsForRoomRequest,
	response *api.QueryMembershipsForRoomResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}

	membershipEventNID, stillInRoom, err := r.DB.GetMembership(ctx, info.RoomNID, request.Sender)
	if err != nil {
		return err
	}

	if membershipEventNID == 0 {
		response.HasBeenInRoom = false
		response.JoinEvents = nil
		return nil
	}

	response.HasBeenInRoom = true
	response.JoinEvents = []gomatrixserverlib.ClientEvent{}

	var events []types.Event
	var stateEntries []types.StateEntry
	if stillInRoom {
		var eventNIDs []types.EventNID
		eventNIDs, err = r.DB.GetMembershipEventNIDsForRoom(ctx, info.RoomNID, request.JoinedOnly, false)
		if err != nil {
			return err
		}

		events, err = r.DB.Events(ctx, eventNIDs)
	} else {
		stateEntries, err = helpers.StateBeforeEvent(ctx, r.DB, *info, membershipEventNID)
		if err != nil {
			logrus.WithField("membership_event_nid", membershipEventNID).WithError(err).Error("failed to load state before event")
			return err
		}
		events, err = helpers.GetMembershipsAtState(ctx, r.DB, stateEntries, request.JoinedOnly)
	}

	if err != nil {
		return err
	}

	for _, event := range events {
		clientEvent := gomatrixserverlib.ToClientEvent(event.Event, gomatrixserverlib.FormatAll)
		response.JoinEvents = append(response.JoinEvents, clientEvent)
	}

	return nil
}

// QueryServerAllowedToSeeEvent implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryServerAllowedToSeeEvent(
	ctx context.Context,
	request *api.QueryServerAllowedToSeeEventRequest,
	response *api.QueryServerAllowedToSeeEventResponse,
) (err error) {
	events, err := r.DB.EventsFromIDs(ctx, []string{request.EventID})
	if err != nil {
		return
	}
	if len(events) == 0 {
		response.AllowedToSeeEvent = false // event doesn't exist so not allowed to see
		return
	}
	roomID := events[0].RoomID()
	isServerInRoom, err := helpers.IsServerCurrentlyInRoom(ctx, r.DB, request.ServerName, roomID)
	if err != nil {
		return
	}
	info, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("QueryServerAllowedToSeeEvent: no room info for room %s", roomID)
	}
	response.AllowedToSeeEvent, err = helpers.CheckServerAllowedToSeeEvent(
		ctx, r.DB, *info, request.EventID, request.ServerName, isServerInRoom,
	)
	return
}

// QueryMissingEvents implements api.RoomserverInternalAPI
// nolint:gocyclo
func (r *RoomserverInternalAPI) QueryMissingEvents(
	ctx context.Context,
	request *api.QueryMissingEventsRequest,
	response *api.QueryMissingEventsResponse,
) error {
	var front []string
	eventsToFilter := make(map[string]bool, len(request.LatestEvents))
	visited := make(map[string]bool, request.Limit) // request.Limit acts as a hint to size.
	for _, id := range request.EarliestEvents {
		visited[id] = true
	}

	for _, id := range request.LatestEvents {
		if !visited[id] {
			front = append(front, id)
			eventsToFilter[id] = true
		}
	}
	events, err := r.DB.EventsFromIDs(ctx, front)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil // we are missing the events being asked to search from, give up.
	}
	info, err := r.DB.RoomInfo(ctx, events[0].RoomID())
	if err != nil {
		return err
	}
	if info == nil || info.IsStub {
		return fmt.Errorf("missing RoomInfo for room %s", events[0].RoomID())
	}

	resultNIDs, err := helpers.ScanEventTree(ctx, r.DB, *info, front, visited, request.Limit, request.ServerName)
	if err != nil {
		return err
	}

	loadedEvents, err := helpers.LoadEvents(ctx, r.DB, resultNIDs)
	if err != nil {
		return err
	}

	response.Events = make([]gomatrixserverlib.HeaderedEvent, 0, len(loadedEvents)-len(eventsToFilter))
	for _, event := range loadedEvents {
		if !eventsToFilter[event.EventID()] {
			roomVersion, verr := r.roomVersion(event.RoomID())
			if verr != nil {
				return verr
			}

			response.Events = append(response.Events, event.Headered(roomVersion))
		}
	}

	return err
}

// QueryStateAndAuthChain implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryStateAndAuthChain(
	ctx context.Context,
	request *api.QueryStateAndAuthChainRequest,
	response *api.QueryStateAndAuthChainResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub {
		return nil
	}
	response.RoomExists = true
	response.RoomVersion = info.RoomVersion

	stateEvents, err := r.loadStateAtEventIDs(ctx, *info, request.PrevEventIDs)
	if err != nil {
		return err
	}
	response.PrevEventsExist = true

	// add the auth event IDs for the current state events too
	var authEventIDs []string
	authEventIDs = append(authEventIDs, request.AuthEventIDs...)
	for _, se := range stateEvents {
		authEventIDs = append(authEventIDs, se.AuthEventIDs()...)
	}
	authEventIDs = util.UniqueStrings(authEventIDs) // de-dupe

	authEvents, err := getAuthChain(ctx, r.DB.EventsFromIDs, authEventIDs)
	if err != nil {
		return err
	}

	if request.ResolveState {
		if stateEvents, err = state.ResolveConflictsAdhoc(
			info.RoomVersion, stateEvents, authEvents,
		); err != nil {
			return err
		}
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, event.Headered(info.RoomVersion))
	}

	for _, event := range authEvents {
		response.AuthChainEvents = append(response.AuthChainEvents, event.Headered(info.RoomVersion))
	}

	return err
}

func (r *RoomserverInternalAPI) loadStateAtEventIDs(ctx context.Context, roomInfo types.RoomInfo, eventIDs []string) ([]gomatrixserverlib.Event, error) {
	roomState := state.NewStateResolution(r.DB, roomInfo)
	prevStates, err := r.DB.StateAtEventIDs(ctx, eventIDs)
	if err != nil {
		switch err.(type) {
		case types.MissingEventError:
			return nil, nil
		default:
			return nil, err
		}
	}

	// Look up the currrent state for the requested tuples.
	stateEntries, err := roomState.LoadCombinedStateAfterEvents(
		ctx, prevStates,
	)
	if err != nil {
		return nil, err
	}

	return helpers.LoadStateEvents(ctx, r.DB, stateEntries)
}

type eventsFromIDs func(context.Context, []string) ([]types.Event, error)

// getAuthChain fetches the auth chain for the given auth events. An auth chain
// is the list of all events that are referenced in the auth_events section, and
// all their auth_events, recursively. The returned set of events contain the
// given events. Will *not* error if we don't have all auth events.
func getAuthChain(
	ctx context.Context, fn eventsFromIDs, authEventIDs []string,
) ([]gomatrixserverlib.Event, error) {
	// List of event IDs to fetch. On each pass, these events will be requested
	// from the database and the `eventsToFetch` will be updated with any new
	// events that we have learned about and need to find. When `eventsToFetch`
	// is eventually empty, we should have reached the end of the chain.
	eventsToFetch := authEventIDs
	authEventsMap := make(map[string]gomatrixserverlib.Event)

	for len(eventsToFetch) > 0 {
		// Try to retrieve the events from the database.
		events, err := fn(ctx, eventsToFetch)
		if err != nil {
			return nil, err
		}

		// We've now fetched these events so clear out `eventsToFetch`. Soon we may
		// add newly discovered events to this for the next pass.
		eventsToFetch = eventsToFetch[:0]

		for _, event := range events {
			// Store the event in the event map - this prevents us from requesting it
			// from the database again.
			authEventsMap[event.EventID()] = event.Event

			// Extract all of the auth events from the newly obtained event. If we
			// don't already have a record of the event, record it in the list of
			// events we want to request for the next pass.
			for _, authEvent := range event.AuthEvents() {
				if _, ok := authEventsMap[authEvent.EventID]; !ok {
					eventsToFetch = append(eventsToFetch, authEvent.EventID)
				}
			}
		}
	}

	// We've now retrieved all of the events we can. Flatten them down into an
	// array and return them.
	var authEvents []gomatrixserverlib.Event
	for _, event := range authEventsMap {
		authEvents = append(authEvents, event)
	}

	return authEvents, nil
}

// QueryRoomVersionCapabilities implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryRoomVersionCapabilities(
	ctx context.Context,
	request *api.QueryRoomVersionCapabilitiesRequest,
	response *api.QueryRoomVersionCapabilitiesResponse,
) error {
	response.DefaultRoomVersion = version.DefaultRoomVersion()
	response.AvailableRoomVersions = make(map[gomatrixserverlib.RoomVersion]string)
	for v, desc := range version.SupportedRoomVersions() {
		if desc.Stable {
			response.AvailableRoomVersions[v] = "stable"
		} else {
			response.AvailableRoomVersions[v] = "unstable"
		}
	}
	return nil
}

// QueryRoomVersionCapabilities implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryRoomVersionForRoom(
	ctx context.Context,
	request *api.QueryRoomVersionForRoomRequest,
	response *api.QueryRoomVersionForRoomResponse,
) error {
	if roomVersion, ok := r.Cache.GetRoomVersion(request.RoomID); ok {
		response.RoomVersion = roomVersion
		return nil
	}

	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("QueryRoomVersionForRoom: missing room info for room %s", request.RoomID)
	}
	response.RoomVersion = info.RoomVersion
	r.Cache.StoreRoomVersion(request.RoomID, response.RoomVersion)
	return nil
}

func (r *RoomserverInternalAPI) roomVersion(roomID string) (gomatrixserverlib.RoomVersion, error) {
	var res api.QueryRoomVersionForRoomResponse
	err := r.QueryRoomVersionForRoom(context.Background(), &api.QueryRoomVersionForRoomRequest{
		RoomID: roomID,
	}, &res)
	return res.RoomVersion, err
}

func (r *RoomserverInternalAPI) QueryPublishedRooms(
	ctx context.Context,
	req *api.QueryPublishedRoomsRequest,
	res *api.QueryPublishedRoomsResponse,
) error {
	rooms, err := r.DB.GetPublishedRooms(ctx)
	if err != nil {
		return err
	}
	res.RoomIDs = rooms
	return nil
}
