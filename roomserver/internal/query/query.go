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

package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/roomserver/version"
)

type Queryer struct {
	DB                storage.Database
	Cache             caching.RoomServerCaches
	IsLocalServerName func(gomatrixserverlib.ServerName) bool
	ServerACLs        *acls.ServerACLs
}

// QueryLatestEventsAndState implements api.RoomserverInternalAPI
func (r *Queryer) QueryLatestEventsAndState(
	ctx context.Context,
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	return helpers.QueryLatestEventsAndState(ctx, r.DB, request, response)
}

// QueryStateAfterEvents implements api.RoomserverInternalAPI
func (r *Queryer) QueryStateAfterEvents(
	ctx context.Context,
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub() {
		return nil
	}

	roomState := state.NewStateResolution(r.DB, info)
	response.RoomExists = true
	response.RoomVersion = info.RoomVersion

	prevStates, err := r.DB.StateAtEventIDs(ctx, request.PrevEventIDs)
	if err != nil {
		if _, ok := err.(types.MissingEventError); ok {
			return nil
		}
		return err
	}
	response.PrevEventsExist = true

	var stateEntries []types.StateEntry
	if len(request.StateToFetch) == 0 {
		// Look up all of the current room state.
		stateEntries, err = roomState.LoadCombinedStateAfterEvents(
			ctx, prevStates,
		)
	} else {
		// Look up the current state for the requested tuples.
		stateEntries, err = roomState.LoadStateAfterEventsForStringTuples(
			ctx, prevStates, request.StateToFetch,
		)
	}
	if err != nil {
		if _, ok := err.(types.MissingEventError); ok {
			return nil
		}
		if _, ok := err.(types.MissingStateError); ok {
			return nil
		}
		return err
	}

	stateEvents, err := helpers.LoadStateEvents(ctx, r.DB, stateEntries)
	if err != nil {
		return err
	}

	if len(request.PrevEventIDs) > 1 {
		var authEventIDs []string
		for _, e := range stateEvents {
			authEventIDs = append(authEventIDs, e.AuthEventIDs()...)
		}
		authEventIDs = util.UniqueStrings(authEventIDs)

		authEvents, err := GetAuthChain(ctx, r.DB.EventsFromIDs, authEventIDs)
		if err != nil {
			return fmt.Errorf("getAuthChain: %w", err)
		}

		stateEvents, err = gomatrixserverlib.ResolveConflicts(info.RoomVersion, stateEvents, authEvents)
		if err != nil {
			return fmt.Errorf("state.ResolveConflictsAdhoc: %w", err)
		}
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, event.Headered(info.RoomVersion))
	}

	return nil
}

// QueryEventsByID implements api.RoomserverInternalAPI
func (r *Queryer) QueryEventsByID(
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
func (r *Queryer) QueryMembershipForUser(
	ctx context.Context,
	request *api.QueryMembershipForUserRequest,
	response *api.QueryMembershipForUserResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil {
		response.RoomExists = false
		return nil
	}
	response.RoomExists = true

	membershipEventNID, stillInRoom, isRoomforgotten, err := r.DB.GetMembership(ctx, info.RoomNID, request.UserID)
	if err != nil {
		return err
	}

	response.IsRoomForgotten = isRoomforgotten

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

// QueryMembershipAtEvent returns the known memberships at a given event.
// If the state before an event is not known, an empty list will be returned
// for that event instead.
func (r *Queryer) QueryMembershipAtEvent(
	ctx context.Context,
	request *api.QueryMembershipAtEventRequest,
	response *api.QueryMembershipAtEventResponse,
) error {
	response.Memberships = make(map[string][]*gomatrixserverlib.HeaderedEvent)
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return fmt.Errorf("unable to get roomInfo: %w", err)
	}
	if info == nil {
		return fmt.Errorf("no roomInfo found")
	}

	// get the users stateKeyNID
	stateKeyNIDs, err := r.DB.EventStateKeyNIDs(ctx, []string{request.UserID})
	if err != nil {
		return fmt.Errorf("unable to get stateKeyNIDs for %s: %w", request.UserID, err)
	}
	if _, ok := stateKeyNIDs[request.UserID]; !ok {
		return fmt.Errorf("requested stateKeyNID for %s was not found", request.UserID)
	}

	stateEntries, err := helpers.MembershipAtEvent(ctx, r.DB, info, request.EventIDs, stateKeyNIDs[request.UserID])
	if err != nil {
		return fmt.Errorf("unable to get state before event: %w", err)
	}

	// If we only have one or less state entries, we can short circuit the below
	// loop and avoid hitting the database
	allStateEventNIDs := make(map[types.EventNID]types.StateEntry)
	for _, eventID := range request.EventIDs {
		stateEntry := stateEntries[eventID]
		for _, s := range stateEntry {
			allStateEventNIDs[s.EventNID] = s
		}
	}

	var canShortCircuit bool
	if len(allStateEventNIDs) <= 1 {
		canShortCircuit = true
	}

	var memberships []types.Event
	for _, eventID := range request.EventIDs {
		stateEntry, ok := stateEntries[eventID]
		if !ok || len(stateEntry) == 0 {
			response.Memberships[eventID] = []*gomatrixserverlib.HeaderedEvent{}
			continue
		}

		// If we can short circuit, e.g. we only have 0 or 1 membership events, we only get the memberships
		// once. If we have more than one membership event, we need to get the state for each state entry.
		if canShortCircuit {
			if len(memberships) == 0 {
				memberships, err = helpers.GetMembershipsAtState(ctx, r.DB, stateEntry, false)
			}
		} else {
			memberships, err = helpers.GetMembershipsAtState(ctx, r.DB, stateEntry, false)
		}
		if err != nil {
			return fmt.Errorf("unable to get memberships at state: %w", err)
		}

		res := make([]*gomatrixserverlib.HeaderedEvent, 0, len(memberships))

		for i := range memberships {
			ev := memberships[i]
			if ev.Type() == gomatrixserverlib.MRoomMember && ev.StateKeyEquals(request.UserID) {
				res = append(res, ev.Headered(info.RoomVersion))
			}
		}
		response.Memberships[eventID] = res
	}

	return nil
}

// QueryMembershipsForRoom implements api.RoomserverInternalAPI
func (r *Queryer) QueryMembershipsForRoom(
	ctx context.Context,
	request *api.QueryMembershipsForRoomRequest,
	response *api.QueryMembershipsForRoomResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil {
		return nil
	}

	// If no sender is specified then we will just return the entire
	// set of memberships for the room, regardless of whether a specific
	// user is allowed to see them or not.
	if request.Sender == "" {
		var events []types.Event
		var eventNIDs []types.EventNID
		eventNIDs, err = r.DB.GetMembershipEventNIDsForRoom(ctx, info.RoomNID, request.JoinedOnly, request.LocalOnly)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return fmt.Errorf("r.DB.GetMembershipEventNIDsForRoom: %w", err)
		}
		events, err = r.DB.Events(ctx, eventNIDs)
		if err != nil {
			return fmt.Errorf("r.DB.Events: %w", err)
		}
		for _, event := range events {
			clientEvent := gomatrixserverlib.ToClientEvent(event.Event, gomatrixserverlib.FormatAll)
			response.JoinEvents = append(response.JoinEvents, clientEvent)
		}
		return nil
	}

	membershipEventNID, stillInRoom, isRoomforgotten, err := r.DB.GetMembership(ctx, info.RoomNID, request.Sender)
	if err != nil {
		return err
	}

	response.IsRoomForgotten = isRoomforgotten

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
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}

		events, err = r.DB.Events(ctx, eventNIDs)
	} else {
		stateEntries, err = helpers.StateBeforeEvent(ctx, r.DB, info, membershipEventNID)
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

// QueryServerJoinedToRoom implements api.RoomserverInternalAPI
func (r *Queryer) QueryServerJoinedToRoom(
	ctx context.Context,
	request *api.QueryServerJoinedToRoomRequest,
	response *api.QueryServerJoinedToRoomResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return fmt.Errorf("r.DB.RoomInfo: %w", err)
	}
	if info == nil || info.IsStub() {
		return nil
	}
	response.RoomExists = true

	if r.IsLocalServerName(request.ServerName) || request.ServerName == "" {
		response.IsInRoom, err = r.DB.GetLocalServerInRoom(ctx, info.RoomNID)
		if err != nil {
			return fmt.Errorf("r.DB.GetLocalServerInRoom: %w", err)
		}
	} else {
		response.IsInRoom, err = r.DB.GetServerInRoom(ctx, info.RoomNID, request.ServerName)
		if err != nil {
			return fmt.Errorf("r.DB.GetServerInRoom: %w", err)
		}
	}

	return nil
}

// QueryServerAllowedToSeeEvent implements api.RoomserverInternalAPI
func (r *Queryer) QueryServerAllowedToSeeEvent(
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

	inRoomReq := &api.QueryServerJoinedToRoomRequest{
		RoomID:     roomID,
		ServerName: request.ServerName,
	}
	inRoomRes := &api.QueryServerJoinedToRoomResponse{}
	if err = r.QueryServerJoinedToRoom(ctx, inRoomReq, inRoomRes); err != nil {
		return fmt.Errorf("r.Queryer.QueryServerJoinedToRoom: %w", err)
	}

	info, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub() {
		return nil
	}
	response.AllowedToSeeEvent, err = helpers.CheckServerAllowedToSeeEvent(
		ctx, r.DB, info, request.EventID, request.ServerName, inRoomRes.IsInRoom,
	)
	return
}

// QueryMissingEvents implements api.RoomserverInternalAPI
func (r *Queryer) QueryMissingEvents(
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
	if info == nil || info.IsStub() {
		return fmt.Errorf("missing RoomInfo for room %s", events[0].RoomID())
	}

	resultNIDs, redactEventIDs, err := helpers.ScanEventTree(ctx, r.DB, info, front, visited, request.Limit, request.ServerName)
	if err != nil {
		return err
	}

	loadedEvents, err := helpers.LoadEvents(ctx, r.DB, resultNIDs)
	if err != nil {
		return err
	}

	response.Events = make([]*gomatrixserverlib.HeaderedEvent, 0, len(loadedEvents)-len(eventsToFilter))
	for _, event := range loadedEvents {
		if !eventsToFilter[event.EventID()] {
			roomVersion, verr := r.roomVersion(event.RoomID())
			if verr != nil {
				return verr
			}
			if _, ok := redactEventIDs[event.EventID()]; ok {
				event.Redact()
			}
			response.Events = append(response.Events, event.Headered(roomVersion))
		}
	}

	return err
}

// QueryStateAndAuthChain implements api.RoomserverInternalAPI
func (r *Queryer) QueryStateAndAuthChain(
	ctx context.Context,
	request *api.QueryStateAndAuthChainRequest,
	response *api.QueryStateAndAuthChainResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub() {
		return nil
	}
	response.RoomExists = true
	response.RoomVersion = info.RoomVersion

	// handle this entirely separately to the other case so we don't have to pull out
	// the entire current state of the room
	// TODO: this probably means it should be a different query operation...
	if request.OnlyFetchAuthChain {
		var authEvents []*gomatrixserverlib.Event
		authEvents, err = GetAuthChain(ctx, r.DB.EventsFromIDs, request.AuthEventIDs)
		if err != nil {
			return err
		}
		for _, event := range authEvents {
			response.AuthChainEvents = append(response.AuthChainEvents, event.Headered(info.RoomVersion))
		}
		return nil
	}

	var stateEvents []*gomatrixserverlib.Event
	stateEvents, rejected, stateMissing, err := r.loadStateAtEventIDs(ctx, info, request.PrevEventIDs)
	if err != nil {
		return err
	}
	response.StateKnown = !stateMissing
	response.IsRejected = rejected
	response.PrevEventsExist = true

	// add the auth event IDs for the current state events too
	var authEventIDs []string
	authEventIDs = append(authEventIDs, request.AuthEventIDs...)
	for _, se := range stateEvents {
		authEventIDs = append(authEventIDs, se.AuthEventIDs()...)
	}
	authEventIDs = util.UniqueStrings(authEventIDs) // de-dupe

	authEvents, err := GetAuthChain(ctx, r.DB.EventsFromIDs, authEventIDs)
	if err != nil {
		return err
	}

	if request.ResolveState {
		if stateEvents, err = gomatrixserverlib.ResolveConflicts(
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

// first bool: is rejected, second bool: state missing
func (r *Queryer) loadStateAtEventIDs(ctx context.Context, roomInfo *types.RoomInfo, eventIDs []string) ([]*gomatrixserverlib.Event, bool, bool, error) {
	roomState := state.NewStateResolution(r.DB, roomInfo)
	prevStates, err := r.DB.StateAtEventIDs(ctx, eventIDs)
	if err != nil {
		switch err.(type) {
		case types.MissingEventError:
			return nil, false, true, nil
		case types.MissingStateError:
			return nil, false, true, nil
		default:
			return nil, false, false, err
		}
	}
	// Currently only used on /state and /state_ids
	rejected := false
	for i := range prevStates {
		if prevStates[i].IsRejected {
			rejected = true
			break
		}
	}

	// Look up the currrent state for the requested tuples.
	stateEntries, err := roomState.LoadCombinedStateAfterEvents(
		ctx, prevStates,
	)
	if err != nil {
		return nil, rejected, false, err
	}

	events, err := helpers.LoadStateEvents(ctx, r.DB, stateEntries)
	return events, rejected, false, err
}

type eventsFromIDs func(context.Context, []string) ([]types.Event, error)

// GetAuthChain fetches the auth chain for the given auth events. An auth chain
// is the list of all events that are referenced in the auth_events section, and
// all their auth_events, recursively. The returned set of events contain the
// given events. Will *not* error if we don't have all auth events.
func GetAuthChain(
	ctx context.Context, fn eventsFromIDs, authEventIDs []string,
) ([]*gomatrixserverlib.Event, error) {
	// List of event IDs to fetch. On each pass, these events will be requested
	// from the database and the `eventsToFetch` will be updated with any new
	// events that we have learned about and need to find. When `eventsToFetch`
	// is eventually empty, we should have reached the end of the chain.
	eventsToFetch := authEventIDs
	authEventsMap := make(map[string]*gomatrixserverlib.Event)

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
	var authEvents []*gomatrixserverlib.Event
	for _, event := range authEventsMap {
		authEvents = append(authEvents, event)
	}

	return authEvents, nil
}

// QueryRoomVersionCapabilities implements api.RoomserverInternalAPI
func (r *Queryer) QueryRoomVersionCapabilities(
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
func (r *Queryer) QueryRoomVersionForRoom(
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

func (r *Queryer) roomVersion(roomID string) (gomatrixserverlib.RoomVersion, error) {
	var res api.QueryRoomVersionForRoomResponse
	err := r.QueryRoomVersionForRoom(context.Background(), &api.QueryRoomVersionForRoomRequest{
		RoomID: roomID,
	}, &res)
	return res.RoomVersion, err
}

func (r *Queryer) QueryPublishedRooms(
	ctx context.Context,
	req *api.QueryPublishedRoomsRequest,
	res *api.QueryPublishedRoomsResponse,
) error {
	if req.RoomID != "" {
		visible, err := r.DB.GetPublishedRoom(ctx, req.RoomID)
		if err == nil && visible {
			res.RoomIDs = []string{req.RoomID}
			return nil
		}
		return err
	}
	rooms, err := r.DB.GetPublishedRooms(ctx, req.NetworkID, req.IncludeAllNetworks)
	if err != nil {
		return err
	}
	res.RoomIDs = rooms
	return nil
}

func (r *Queryer) QueryCurrentState(ctx context.Context, req *api.QueryCurrentStateRequest, res *api.QueryCurrentStateResponse) error {
	res.StateEvents = make(map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent)
	for _, tuple := range req.StateTuples {
		if tuple.StateKey == "*" && req.AllowWildcards {
			events, err := r.DB.GetStateEventsWithEventType(ctx, req.RoomID, tuple.EventType)
			if err != nil {
				return err
			}
			for _, e := range events {
				res.StateEvents[gomatrixserverlib.StateKeyTuple{
					EventType: e.Type(),
					StateKey:  *e.StateKey(),
				}] = e
			}
		} else {
			ev, err := r.DB.GetStateEvent(ctx, req.RoomID, tuple.EventType, tuple.StateKey)
			if err != nil {
				return err
			}
			if ev != nil {
				res.StateEvents[tuple] = ev
			}
		}
	}
	return nil
}

func (r *Queryer) QueryRoomsForUser(ctx context.Context, req *api.QueryRoomsForUserRequest, res *api.QueryRoomsForUserResponse) error {
	roomIDs, err := r.DB.GetRoomsByMembership(ctx, req.UserID, req.WantMembership)
	if err != nil {
		return err
	}
	res.RoomIDs = roomIDs
	return nil
}

func (r *Queryer) QueryKnownUsers(ctx context.Context, req *api.QueryKnownUsersRequest, res *api.QueryKnownUsersResponse) error {
	users, err := r.DB.GetKnownUsers(ctx, req.UserID, req.SearchString, req.Limit)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	for _, user := range users {
		res.Users = append(res.Users, authtypes.FullyQualifiedProfile{
			UserID: user,
		})
	}
	return nil
}

func (r *Queryer) QueryBulkStateContent(ctx context.Context, req *api.QueryBulkStateContentRequest, res *api.QueryBulkStateContentResponse) error {
	events, err := r.DB.GetBulkStateContent(ctx, req.RoomIDs, req.StateTuples, req.AllowWildcards)
	if err != nil {
		return err
	}
	res.Rooms = make(map[string]map[gomatrixserverlib.StateKeyTuple]string)
	for _, ev := range events {
		if res.Rooms[ev.RoomID] == nil {
			res.Rooms[ev.RoomID] = make(map[gomatrixserverlib.StateKeyTuple]string)
		}
		room := res.Rooms[ev.RoomID]
		room[gomatrixserverlib.StateKeyTuple{
			EventType: ev.EventType,
			StateKey:  ev.StateKey,
		}] = ev.ContentValue
		res.Rooms[ev.RoomID] = room
	}
	return nil
}

func (r *Queryer) QuerySharedUsers(ctx context.Context, req *api.QuerySharedUsersRequest, res *api.QuerySharedUsersResponse) error {
	roomIDs, err := r.DB.GetRoomsByMembership(ctx, req.UserID, "join")
	if err != nil {
		return err
	}
	roomIDs = append(roomIDs, req.IncludeRoomIDs...)
	excludeMap := make(map[string]bool)
	for _, roomID := range req.ExcludeRoomIDs {
		excludeMap[roomID] = true
	}
	// filter out excluded rooms
	j := 0
	for i := range roomIDs {
		// move elements to include to the beginning of the slice
		// then trim elements on the right
		if !excludeMap[roomIDs[i]] {
			roomIDs[j] = roomIDs[i]
			j++
		}
	}
	roomIDs = roomIDs[:j]

	users, err := r.DB.JoinedUsersSetInRooms(ctx, roomIDs, req.OtherUserIDs, req.LocalOnly)
	if err != nil {
		return err
	}
	res.UserIDsToCount = users
	return nil
}

func (r *Queryer) QueryServerBannedFromRoom(ctx context.Context, req *api.QueryServerBannedFromRoomRequest, res *api.QueryServerBannedFromRoomResponse) error {
	if r.ServerACLs == nil {
		return errors.New("no server ACL tracking")
	}
	res.Banned = r.ServerACLs.IsServerBannedFromRoom(req.ServerName, req.RoomID)
	return nil
}

func (r *Queryer) QueryAuthChain(ctx context.Context, req *api.QueryAuthChainRequest, res *api.QueryAuthChainResponse) error {
	chain, err := GetAuthChain(ctx, r.DB.EventsFromIDs, req.EventIDs)
	if err != nil {
		return err
	}
	hchain := make([]*gomatrixserverlib.HeaderedEvent, len(chain))
	for i := range chain {
		hchain[i] = chain[i].Headered(chain[i].Version())
	}
	res.AuthChain = hchain
	return nil
}

// nolint:gocyclo
func (r *Queryer) QueryRestrictedJoinAllowed(ctx context.Context, req *api.QueryRestrictedJoinAllowedRequest, res *api.QueryRestrictedJoinAllowedResponse) error {
	// Look up if we know anything about the room. If it doesn't exist
	// or is a stub entry then we can't do anything.
	roomInfo, err := r.DB.RoomInfo(ctx, req.RoomID)
	if err != nil {
		return fmt.Errorf("r.DB.RoomInfo: %w", err)
	}
	if roomInfo == nil || roomInfo.IsStub() {
		return nil // fmt.Errorf("room %q doesn't exist or is stub room", req.RoomID)
	}
	// If the room version doesn't allow restricted joins then don't
	// try to process any further.
	allowRestrictedJoins, err := roomInfo.RoomVersion.MayAllowRestrictedJoinsInEventAuth()
	if err != nil {
		return fmt.Errorf("roomInfo.RoomVersion.AllowRestrictedJoinsInEventAuth: %w", err)
	} else if !allowRestrictedJoins {
		return nil
	}
	// Start off by populating the "resident" flag in the response. If we
	// come across any rooms in the request that are missing, we will unset
	// the flag.
	res.Resident = true
	// Get the join rules to work out if the join rule is "restricted".
	joinRulesEvent, err := r.DB.GetStateEvent(ctx, req.RoomID, gomatrixserverlib.MRoomJoinRules, "")
	if err != nil {
		return fmt.Errorf("r.DB.GetStateEvent: %w", err)
	}
	if joinRulesEvent == nil {
		return nil
	}
	var joinRules gomatrixserverlib.JoinRuleContent
	if err = json.Unmarshal(joinRulesEvent.Content(), &joinRules); err != nil {
		return fmt.Errorf("json.Unmarshal: %w", err)
	}
	// If the join rule isn't "restricted" then there's nothing more to do.
	res.Restricted = joinRules.JoinRule == gomatrixserverlib.Restricted
	if !res.Restricted {
		return nil
	}
	// If the user is already invited to the room then the join is allowed
	// but we don't specify an authorised via user, since the event auth
	// will allow the join anyway.
	var pending bool
	if pending, _, _, _, err = helpers.IsInvitePending(ctx, r.DB, req.RoomID, req.UserID); err != nil {
		return fmt.Errorf("helpers.IsInvitePending: %w", err)
	} else if pending {
		res.Allowed = true
		return nil
	}
	// We need to get the power levels content so that we can determine which
	// users in the room are entitled to issue invites. We need to use one of
	// these users as the authorising user.
	powerLevelsEvent, err := r.DB.GetStateEvent(ctx, req.RoomID, gomatrixserverlib.MRoomPowerLevels, "")
	if err != nil {
		return fmt.Errorf("r.DB.GetStateEvent: %w", err)
	}
	var powerLevels gomatrixserverlib.PowerLevelContent
	if err = json.Unmarshal(powerLevelsEvent.Content(), &powerLevels); err != nil {
		return fmt.Errorf("json.Unmarshal: %w", err)
	}
	// Step through the join rules and see if the user matches any of them.
	for _, rule := range joinRules.Allow {
		// We only understand "m.room_membership" rules at this point in
		// time, so skip any rule that doesn't match those.
		if rule.Type != gomatrixserverlib.MRoomMembership {
			continue
		}
		// See if the room exists. If it doesn't exist or if it's a stub
		// room entry then we can't check memberships.
		targetRoomInfo, err := r.DB.RoomInfo(ctx, rule.RoomID)
		if err != nil || targetRoomInfo == nil || targetRoomInfo.IsStub() {
			res.Resident = false
			continue
		}
		// First of all work out if *we* are still in the room, otherwise
		// it's possible that the memberships will be out of date.
		isIn, err := r.DB.GetLocalServerInRoom(ctx, targetRoomInfo.RoomNID)
		if err != nil || !isIn {
			// If we aren't in the room, we can no longer tell if the room
			// memberships are up-to-date.
			res.Resident = false
			continue
		}
		// At this point we're happy that we are in the room, so now let's
		// see if the target user is in the room.
		_, isIn, _, err = r.DB.GetMembership(ctx, targetRoomInfo.RoomNID, req.UserID)
		if err != nil {
			continue
		}
		// If the user is not in the room then we will skip them.
		if !isIn {
			continue
		}
		// The user is in the room, so now we will need to authorise the
		// join using the user ID of one of our own users in the room. Pick
		// one.
		joinNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, targetRoomInfo.RoomNID, true, true)
		if err != nil || len(joinNIDs) == 0 {
			// There should always be more than one join NID at this point
			// because we are gated behind GetLocalServerInRoom, but y'know,
			// sometimes strange things happen.
			continue
		}
		// For each of the joined users, let's see if we can get a valid
		// membership event.
		for _, joinNID := range joinNIDs {
			events, err := r.DB.Events(ctx, []types.EventNID{joinNID})
			if err != nil || len(events) != 1 {
				continue
			}
			event := events[0]
			if event.Type() != gomatrixserverlib.MRoomMember || event.StateKey() == nil {
				continue // shouldn't happen
			}
			// Only users that have the power to invite should be chosen.
			if powerLevels.UserLevel(*event.StateKey()) < powerLevels.Invite {
				continue
			}
			res.Resident = true
			res.Allowed = true
			res.AuthorisedVia = *event.StateKey()
			return nil
		}
	}
	return nil
}
