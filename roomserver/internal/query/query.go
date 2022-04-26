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
	"errors"
	"fmt"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type Queryer struct {
	DB         storage.Database
	Cache      caching.RoomServerCaches
	ServerName gomatrixserverlib.ServerName
	ServerACLs *acls.ServerACLs
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
	if info == nil || info.IsStub {
		return nil
	}

	roomState := state.NewStateResolution(r.DB, info)
	response.RoomExists = true
	response.RoomVersion = info.RoomVersion

	prevStates, err := r.DB.StateAtEventIDs(ctx, request.PrevEventIDs)
	if err != nil {
		switch err.(type) {
		case types.MissingEventError:
			util.GetLogger(ctx).Errorf("QueryStateAfterEvents: MissingEventError: %s", err)
			return nil
		default:
			return err
		}
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
		return err
	}

	stateEvents, err := helpers.LoadStateEvents(ctx, r.DB, stateEntries)
	if err != nil {
		return err
	}

	if len(request.PrevEventIDs) > 1 && len(request.StateToFetch) == 0 {
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
		return fmt.Errorf("QueryMembershipForUser: unknown room %s", request.RoomID)
	}

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
	if info == nil || info.IsStub {
		return nil
	}
	response.RoomExists = true

	if request.ServerName == r.ServerName || request.ServerName == "" {
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
	if info == nil {
		return fmt.Errorf("QueryServerAllowedToSeeEvent: no room info for room %s", roomID)
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
	if info == nil || info.IsStub {
		return fmt.Errorf("missing RoomInfo for room %s", events[0].RoomID())
	}

	resultNIDs, err := helpers.ScanEventTree(ctx, r.DB, info, front, visited, request.Limit, request.ServerName)
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
	if info == nil || info.IsStub {
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
	stateEvents, rejected, err := r.loadStateAtEventIDs(ctx, info, request.PrevEventIDs)
	if err != nil {
		return err
	}
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

func (r *Queryer) loadStateAtEventIDs(ctx context.Context, roomInfo *types.RoomInfo, eventIDs []string) ([]*gomatrixserverlib.Event, bool, error) {
	roomState := state.NewStateResolution(r.DB, roomInfo)
	prevStates, err := r.DB.StateAtEventIDs(ctx, eventIDs)
	if err != nil {
		switch err.(type) {
		case types.MissingEventError:
			return nil, false, nil
		default:
			return nil, false, err
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
		return nil, rejected, err
	}

	events, err := helpers.LoadStateEvents(ctx, r.DB, stateEntries)

	return events, rejected, err
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
	rooms, err := r.DB.GetPublishedRooms(ctx)
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
	if err != nil {
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

	users, err := r.DB.JoinedUsersSetInRooms(ctx, roomIDs, req.OtherUserIDs)
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
