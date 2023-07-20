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
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"

	//"github.com/matrix-org/dendrite/roomserver/internal"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/synctypes"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	fsAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/roomserver/acls"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type Queryer struct {
	DB                storage.Database
	Cache             caching.RoomServerCaches
	IsLocalServerName func(spec.ServerName) bool
	ServerACLs        *acls.ServerACLs
	Cfg               *config.Dendrite
	FSAPI             fsAPI.RoomserverFederationAPI
}

func (r *Queryer) RestrictedRoomJoinInfo(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID, localServerName spec.ServerName) (*gomatrixserverlib.RestrictedRoomJoinInfo, error) {
	roomInfo, err := r.QueryRoomInfo(ctx, roomID)
	if err != nil || roomInfo == nil || roomInfo.IsStub() {
		return nil, err
	}

	req := api.QueryServerJoinedToRoomRequest{
		ServerName: localServerName,
		RoomID:     roomID.String(),
	}
	res := api.QueryServerJoinedToRoomResponse{}
	if err = r.QueryServerJoinedToRoom(ctx, &req, &res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("rsAPI.QueryServerJoinedToRoom failed")
		return nil, fmt.Errorf("InternalServerError: Failed to query room: %w", err)
	}

	userJoinedToRoom, err := r.UserJoinedToRoom(ctx, types.RoomNID(roomInfo.RoomNID), senderID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("rsAPI.UserJoinedToRoom failed")
		return nil, fmt.Errorf("InternalServerError: %w", err)
	}

	locallyJoinedUsers, err := r.LocallyJoinedUsers(ctx, roomInfo.RoomVersion, types.RoomNID(roomInfo.RoomNID))
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("rsAPI.GetLocallyJoinedUsers failed")
		return nil, fmt.Errorf("InternalServerError: %w", err)
	}

	return &gomatrixserverlib.RestrictedRoomJoinInfo{
		LocalServerInRoom: res.RoomExists && res.IsInRoom,
		UserJoinedToRoom:  userJoinedToRoom,
		JoinedUsers:       locallyJoinedUsers,
	}, nil
}

// QueryLatestEventsAndState implements api.RoomserverInternalAPI
func (r *Queryer) QueryLatestEventsAndState(
	ctx context.Context,
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	return helpers.QueryLatestEventsAndState(ctx, r.DB, r, request, response)
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

	roomState := state.NewStateResolution(r.DB, info, r)
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

	stateEvents, err := helpers.LoadStateEvents(ctx, r.DB, info, stateEntries)
	if err != nil {
		return err
	}

	if len(request.PrevEventIDs) > 1 {
		var authEventIDs []string
		for _, e := range stateEvents {
			authEventIDs = append(authEventIDs, e.AuthEventIDs()...)
		}
		authEventIDs = util.UniqueStrings(authEventIDs)

		authEvents, err := GetAuthChain(ctx, r.DB.EventsFromIDs, info, authEventIDs)
		if err != nil {
			return fmt.Errorf("getAuthChain: %w", err)
		}

		stateEvents, err = gomatrixserverlib.ResolveConflicts(
			info.RoomVersion, gomatrixserverlib.ToPDUs(stateEvents), gomatrixserverlib.ToPDUs(authEvents), func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
				return r.QueryUserIDForSender(ctx, roomID, senderID)
			},
		)
		if err != nil {
			return fmt.Errorf("state.ResolveConflictsAdhoc: %w", err)
		}
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, &types.HeaderedEvent{PDU: event})
	}

	return nil
}

// QueryEventsByID queries a list of events by event ID for one room. If no room is specified, it will try to determine
// which room to use by querying the first events roomID.
func (r *Queryer) QueryEventsByID(
	ctx context.Context,
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	if len(request.EventIDs) == 0 {
		return nil
	}
	var err error
	// We didn't receive a room ID, we need to fetch it first before we can continue.
	// This happens for e.g. ` /_matrix/federation/v1/event/{eventId}`
	var roomInfo *types.RoomInfo
	if request.RoomID == "" {
		var eventNIDs map[string]types.EventMetadata
		eventNIDs, err = r.DB.EventNIDs(ctx, []string{request.EventIDs[0]})
		if err != nil {
			return err
		}
		if len(eventNIDs) == 0 {
			return nil
		}
		roomInfo, err = r.DB.RoomInfoByNID(ctx, eventNIDs[request.EventIDs[0]].RoomNID)
	} else {
		roomInfo, err = r.DB.RoomInfo(ctx, request.RoomID)
	}
	if err != nil {
		return err
	}
	if roomInfo == nil {
		return nil
	}
	events, err := r.DB.EventsFromIDs(ctx, roomInfo, request.EventIDs)
	if err != nil {
		return err
	}

	for _, event := range events {
		response.Events = append(response.Events, &types.HeaderedEvent{PDU: event.PDU})
	}

	return nil
}

// QueryMembershipForSenderID implements api.RoomserverInternalAPI
func (r *Queryer) QueryMembershipForSenderID(
	ctx context.Context,
	roomID spec.RoomID,
	senderID spec.SenderID,
	response *api.QueryMembershipForUserResponse,
) error {
	info, err := r.DB.RoomInfo(ctx, roomID.String())
	if err != nil {
		return err
	}
	if info == nil {
		response.RoomExists = false
		return nil
	}
	response.RoomExists = true

	membershipEventNID, stillInRoom, isRoomforgotten, err := r.DB.GetMembership(ctx, info.RoomNID, senderID)
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

	evs, err := r.DB.Events(ctx, info.RoomVersion, []types.EventNID{membershipEventNID})
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

// QueryMembershipForUser implements api.RoomserverInternalAPI
func (r *Queryer) QueryMembershipForUser(
	ctx context.Context,
	request *api.QueryMembershipForUserRequest,
	response *api.QueryMembershipForUserResponse,
) error {
	roomID, err := spec.NewRoomID(request.RoomID)
	if err != nil {
		return err
	}
	senderID, err := r.QuerySenderIDForUser(ctx, *roomID, request.UserID)
	if err != nil {
		return err
	}

	return r.QueryMembershipForSenderID(ctx, *roomID, senderID, response)
}

// QueryMembershipAtEvent returns the known memberships at a given event.
// If the state before an event is not known, an empty list will be returned
// for that event instead.
func (r *Queryer) QueryMembershipAtEvent(
	ctx context.Context,
	request *api.QueryMembershipAtEventRequest,
	response *api.QueryMembershipAtEventResponse,
) error {
	response.Membership = make(map[string]*types.HeaderedEvent)

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

	response.Membership, err = r.DB.GetMembershipForHistoryVisibility(ctx, stateKeyNIDs[request.UserID], info, request.EventIDs...)
	switch err {
	case nil:
		return nil
	case tables.OptimisationNotSupportedError: // fallthrough, slow way of getting the membership events for each event
	default:
		return err
	}

	response.Membership = make(map[string]*types.HeaderedEvent)
	stateEntries, err := helpers.MembershipAtEvent(ctx, r.DB, nil, request.EventIDs, stateKeyNIDs[request.UserID], r)
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
			response.Membership[eventID] = nil
			continue
		}

		// If we can short circuit, e.g. we only have 0 or 1 membership events, we only get the memberships
		// once. If we have more than one membership event, we need to get the state for each state entry.
		if canShortCircuit {
			if len(memberships) == 0 {
				memberships, err = helpers.GetMembershipsAtState(ctx, r.DB, info, stateEntry, false)
			}
		} else {
			memberships, err = helpers.GetMembershipsAtState(ctx, r.DB, info, stateEntry, false)
		}
		if err != nil {
			return fmt.Errorf("unable to get memberships at state: %w", err)
		}

		// Iterate over all membership events we got. Given we only query the membership for
		// one user and assuming this user only ever has one membership event associated to
		// a given event, overwrite any other existing membership events.
		for i := range memberships {
			ev := memberships[i]
			if ev.Type() == spec.MRoomMember && ev.StateKeyEquals(request.UserID) {
				response.Membership[eventID] = &types.HeaderedEvent{PDU: ev.PDU}
			}
		}
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
	if request.SenderID == "" {
		var events []types.Event
		var eventNIDs []types.EventNID
		eventNIDs, err = r.DB.GetMembershipEventNIDsForRoom(ctx, info.RoomNID, request.JoinedOnly, request.LocalOnly)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return fmt.Errorf("r.DB.GetMembershipEventNIDsForRoom: %w", err)
		}
		events, err = r.DB.Events(ctx, info.RoomVersion, eventNIDs)
		if err != nil {
			return fmt.Errorf("r.DB.Events: %w", err)
		}
		for _, event := range events {
			clientEvent := synctypes.ToClientEventDefault(func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
				return r.QueryUserIDForSender(ctx, roomID, senderID)
			}, event)
			response.JoinEvents = append(response.JoinEvents, clientEvent)
		}
		return nil
	}

	membershipEventNID, stillInRoom, isRoomforgotten, err := r.DB.GetMembership(ctx, info.RoomNID, request.SenderID)
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
	response.JoinEvents = []synctypes.ClientEvent{}

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

		events, err = r.DB.Events(ctx, info.RoomVersion, eventNIDs)
	} else {
		stateEntries, err = helpers.StateBeforeEvent(ctx, r.DB, info, membershipEventNID, r)
		if err != nil {
			logrus.WithField("membership_event_nid", membershipEventNID).WithError(err).Error("failed to load state before event")
			return err
		}
		events, err = helpers.GetMembershipsAtState(ctx, r.DB, info, stateEntries, request.JoinedOnly)
	}

	if err != nil {
		return err
	}

	for _, event := range events {
		clientEvent := synctypes.ToClientEventDefault(func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return r.QueryUserIDForSender(ctx, roomID, senderID)
		}, event)
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
	if info != nil {
		response.RoomVersion = info.RoomVersion
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
	serverName spec.ServerName,
	eventID string,
	roomID string,
) (allowed bool, err error) {
	events, err := r.DB.EventNIDs(ctx, []string{eventID})
	if err != nil {
		return
	}
	if len(events) == 0 {
		return allowed, nil
	}
	info, err := r.DB.RoomInfoByNID(ctx, events[eventID].RoomNID)
	if err != nil {
		return allowed, err
	}
	if info == nil || info.IsStub() {
		return allowed, nil
	}
	var isInRoom bool
	if r.IsLocalServerName(serverName) || serverName == "" {
		isInRoom, err = r.DB.GetLocalServerInRoom(ctx, info.RoomNID)
		if err != nil {
			return allowed, fmt.Errorf("r.DB.GetLocalServerInRoom: %w", err)
		}
	} else {
		isInRoom, err = r.DB.GetServerInRoom(ctx, info.RoomNID, serverName)
		if err != nil {
			return allowed, fmt.Errorf("r.DB.GetServerInRoom: %w", err)
		}
	}

	return helpers.CheckServerAllowedToSeeEvent(
		ctx, r.DB, info, roomID, eventID, serverName, isInRoom, r,
	)
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
	if len(front) == 0 {
		return nil // no events to query, give up.
	}
	events, err := r.DB.EventNIDs(ctx, []string{front[0]})
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil // we are missing the events being asked to search from, give up.
	}
	info, err := r.DB.RoomInfoByNID(ctx, events[front[0]].RoomNID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub() {
		return fmt.Errorf("missing RoomInfo for room %d", events[front[0]].RoomNID)
	}

	resultNIDs, redactEventIDs, err := helpers.ScanEventTree(ctx, r.DB, info, front, visited, request.Limit, request.ServerName, r)
	if err != nil {
		return err
	}

	loadedEvents, err := helpers.LoadEvents(ctx, r.DB, info, resultNIDs)
	if err != nil {
		return err
	}

	response.Events = make([]*types.HeaderedEvent, 0, len(loadedEvents)-len(eventsToFilter))
	for _, event := range loadedEvents {
		if !eventsToFilter[event.EventID()] {
			if _, ok := redactEventIDs[event.EventID()]; ok {
				event.Redact()
			}
			response.Events = append(response.Events, &types.HeaderedEvent{PDU: event})
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
		var authEvents []gomatrixserverlib.PDU
		authEvents, err = GetAuthChain(ctx, r.DB.EventsFromIDs, info, request.AuthEventIDs)
		if err != nil {
			return err
		}
		for _, event := range authEvents {
			response.AuthChainEvents = append(response.AuthChainEvents, &types.HeaderedEvent{PDU: event})
		}
		return nil
	}

	var stateEvents []gomatrixserverlib.PDU
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

	authEvents, err := GetAuthChain(ctx, r.DB.EventsFromIDs, info, authEventIDs)
	if err != nil {
		return err
	}

	if request.ResolveState {
		stateEvents, err = gomatrixserverlib.ResolveConflicts(
			info.RoomVersion, gomatrixserverlib.ToPDUs(stateEvents), gomatrixserverlib.ToPDUs(authEvents), func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
				return r.QueryUserIDForSender(ctx, roomID, senderID)
			},
		)
		if err != nil {
			return err
		}
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, &types.HeaderedEvent{PDU: event})
	}

	for _, event := range authEvents {
		response.AuthChainEvents = append(response.AuthChainEvents, &types.HeaderedEvent{PDU: event})
	}

	return err
}

// first bool: is rejected, second bool: state missing
func (r *Queryer) loadStateAtEventIDs(ctx context.Context, roomInfo *types.RoomInfo, eventIDs []string) ([]gomatrixserverlib.PDU, bool, bool, error) {
	roomState := state.NewStateResolution(r.DB, roomInfo, r)
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

	events, err := helpers.LoadStateEvents(ctx, r.DB, roomInfo, stateEntries)
	return events, rejected, false, err
}

type eventsFromIDs func(context.Context, *types.RoomInfo, []string) ([]types.Event, error)

// GetAuthChain fetches the auth chain for the given auth events. An auth chain
// is the list of all events that are referenced in the auth_events section, and
// all their auth_events, recursively. The returned set of events contain the
// given events. Will *not* error if we don't have all auth events.
func GetAuthChain(
	ctx context.Context, fn eventsFromIDs, roomInfo *types.RoomInfo, authEventIDs []string,
) ([]gomatrixserverlib.PDU, error) {
	// List of event IDs to fetch. On each pass, these events will be requested
	// from the database and the `eventsToFetch` will be updated with any new
	// events that we have learned about and need to find. When `eventsToFetch`
	// is eventually empty, we should have reached the end of the chain.
	eventsToFetch := authEventIDs
	authEventsMap := make(map[string]gomatrixserverlib.PDU)

	for len(eventsToFetch) > 0 {
		// Try to retrieve the events from the database.
		events, err := fn(ctx, roomInfo, eventsToFetch)
		if err != nil {
			return nil, err
		}

		// We've now fetched these events so clear out `eventsToFetch`. Soon we may
		// add newly discovered events to this for the next pass.
		eventsToFetch = eventsToFetch[:0]

		for _, event := range events {
			// Store the event in the event map - this prevents us from requesting it
			// from the database again.
			authEventsMap[event.EventID()] = event.PDU

			// Extract all of the auth events from the newly obtained event. If we
			// don't already have a record of the event, record it in the list of
			// events we want to request for the next pass.
			for _, authEventID := range event.AuthEventIDs() {
				if _, ok := authEventsMap[authEventID]; !ok {
					eventsToFetch = append(eventsToFetch, authEventID)
				}
			}
		}
	}

	// We've now retrieved all of the events we can. Flatten them down into an
	// array and return them.
	var authEvents []gomatrixserverlib.PDU
	for _, event := range authEventsMap {
		authEvents = append(authEvents, event)
	}

	return authEvents, nil
}

// QueryRoomVersionForRoom implements api.RoomserverInternalAPI
func (r *Queryer) QueryRoomVersionForRoom(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error) {
	if roomVersion, ok := r.Cache.GetRoomVersion(roomID); ok {
		return roomVersion, nil
	}

	info, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", fmt.Errorf("QueryRoomVersionForRoom: missing room info for room %s", roomID)
	}
	r.Cache.StoreRoomVersion(roomID, info.RoomVersion)
	return info.RoomVersion, nil
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
	res.StateEvents = make(map[gomatrixserverlib.StateKeyTuple]*types.HeaderedEvent)
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

func (r *Queryer) QueryLeftUsers(ctx context.Context, req *api.QueryLeftUsersRequest, res *api.QueryLeftUsersResponse) error {
	var err error
	res.LeftUsers, err = r.DB.GetLeftUsers(ctx, req.StaleDeviceListUsers)
	return err
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
	chain, err := GetAuthChain(ctx, r.DB.EventsFromIDs, nil, req.EventIDs)
	if err != nil {
		return err
	}
	hchain := make([]*types.HeaderedEvent, len(chain))
	for i := range chain {
		hchain[i] = &types.HeaderedEvent{PDU: chain[i]}
	}
	res.AuthChain = hchain
	return nil
}

func (r *Queryer) InvitePending(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (bool, error) {
	pending, _, _, _, err := helpers.IsInvitePending(ctx, r.DB, roomID.String(), senderID)
	return pending, err
}

func (r *Queryer) QueryRoomInfo(ctx context.Context, roomID spec.RoomID) (*types.RoomInfo, error) {
	return r.DB.RoomInfo(ctx, roomID.String())
}

func (r *Queryer) CurrentStateEvent(ctx context.Context, roomID spec.RoomID, eventType string, stateKey string) (gomatrixserverlib.PDU, error) {
	res, err := r.DB.GetStateEvent(ctx, roomID.String(), eventType, stateKey)
	if res == nil {
		return nil, err
	}
	return res, err
}

func (r *Queryer) UserJoinedToRoom(ctx context.Context, roomNID types.RoomNID, senderID spec.SenderID) (bool, error) {
	_, isIn, _, err := r.DB.GetMembership(ctx, roomNID, senderID)
	return isIn, err
}

func (r *Queryer) LocallyJoinedUsers(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, roomNID types.RoomNID) ([]gomatrixserverlib.PDU, error) {
	joinNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, roomNID, true, true)
	if err != nil {
		return nil, err
	}

	events, err := r.DB.Events(ctx, roomVersion, joinNIDs)
	if err != nil {
		return nil, err
	}

	// For each of the joined users, let's see if we can get a valid
	// membership event.
	joinedUsers := []gomatrixserverlib.PDU{}
	for _, event := range events {
		if event.Type() != spec.MRoomMember || event.StateKey() == nil {
			continue // shouldn't happen
		}

		joinedUsers = append(joinedUsers, event)
	}

	return joinedUsers, nil
}

func (r *Queryer) JoinedUserCount(ctx context.Context, roomID string) (int, error) {
	info, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, nil
	}

	// TODO: this can be further optimised by just using a SELECT COUNT query
	nids, err := r.DB.GetMembershipEventNIDsForRoom(ctx, info.RoomNID, true, false)
	return len(nids), err
}

// nolint:gocyclo
func (r *Queryer) QueryRestrictedJoinAllowed(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (string, error) {
	// Look up if we know anything about the room. If it doesn't exist
	// or is a stub entry then we can't do anything.
	roomInfo, err := r.DB.RoomInfo(ctx, roomID.String())
	if err != nil {
		return "", fmt.Errorf("r.DB.RoomInfo: %w", err)
	}
	if roomInfo == nil || roomInfo.IsStub() {
		return "", nil // fmt.Errorf("room %q doesn't exist or is stub room", req.RoomID)
	}
	verImpl, err := gomatrixserverlib.GetRoomVersion(roomInfo.RoomVersion)
	if err != nil {
		return "", err
	}

	return verImpl.CheckRestrictedJoin(ctx, r.Cfg.Global.ServerName, &api.JoinRoomQuerier{Roomserver: r}, roomID, senderID)
}

func (r *Queryer) QuerySenderIDForUser(ctx context.Context, roomID spec.RoomID, userID spec.UserID) (spec.SenderID, error) {
	version, err := r.DB.GetRoomVersion(ctx, roomID.String())
	if err != nil {
		return "", err
	}

	switch version {
	case gomatrixserverlib.RoomVersionPseudoIDs:
		key, err := r.DB.SelectUserRoomPublicKey(ctx, userID, roomID)
		if err != nil {
			return "", err
		}
		return spec.SenderID(spec.Base64Bytes(key).Encode()), nil
	default:
		return spec.SenderID(userID.String()), nil
	}
}

func (r *Queryer) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	userID, err := spec.NewUserID(string(senderID), true)
	if err == nil {
		return userID, nil
	}

	bytes := spec.Base64Bytes{}
	err = bytes.Decode(string(senderID))
	if err != nil {
		return nil, err
	}
	queryMap := map[spec.RoomID][]ed25519.PublicKey{roomID: {ed25519.PublicKey(bytes)}}
	result, err := r.DB.SelectUserIDsForPublicKeys(ctx, queryMap)
	if err != nil {
		return nil, err
	}

	if userKeys, ok := result[roomID]; ok {
		if userID, ok := userKeys[string(senderID)]; ok {
			return spec.NewUserID(userID, true)
		}
	}

	return nil, nil
}
