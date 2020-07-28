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

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/auth"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
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
	roomVersion, err := r.DB.GetRoomVersionForRoom(ctx, request.RoomID)
	if err != nil {
		response.RoomExists = false
		return nil
	}

	roomState := state.NewStateResolution(r.DB)

	roomNID, err := r.DB.RoomNIDExcludingStubs(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true
	response.RoomVersion = roomVersion

	var currentStateSnapshotNID types.StateSnapshotNID
	response.LatestEvents, currentStateSnapshotNID, response.Depth, err =
		r.DB.LatestEventIDs(ctx, roomNID)
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

	stateEvents, err := r.loadStateEvents(ctx, stateEntries)
	if err != nil {
		return err
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, event.Headered(roomVersion))
	}

	return nil
}

// QueryStateAfterEvents implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	roomVersion, err := r.DB.GetRoomVersionForRoom(ctx, request.RoomID)
	if err != nil {
		response.RoomExists = false
		return nil
	}

	roomState := state.NewStateResolution(r.DB)

	roomNID, err := r.DB.RoomNIDExcludingStubs(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true
	response.RoomVersion = roomVersion

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
		ctx, roomNID, prevStates, request.StateToFetch,
	)
	if err != nil {
		return err
	}

	stateEvents, err := r.loadStateEvents(ctx, stateEntries)
	if err != nil {
		return err
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, event.Headered(roomVersion))
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

	events, err := r.loadEvents(ctx, eventNIDs)
	if err != nil {
		return err
	}

	for _, event := range events {
		roomVersion, verr := r.DB.GetRoomVersionForRoom(ctx, event.RoomID())
		if verr != nil {
			return verr
		}

		response.Events = append(response.Events, event.Headered(roomVersion))
	}

	return nil
}

func (r *RoomserverInternalAPI) loadStateEvents(
	ctx context.Context, stateEntries []types.StateEntry,
) ([]gomatrixserverlib.Event, error) {
	eventNIDs := make([]types.EventNID, len(stateEntries))
	for i := range stateEntries {
		eventNIDs[i] = stateEntries[i].EventNID
	}
	return r.loadEvents(ctx, eventNIDs)
}

func (r *RoomserverInternalAPI) loadEvents(
	ctx context.Context, eventNIDs []types.EventNID,
) ([]gomatrixserverlib.Event, error) {
	stateEvents, err := r.DB.Events(ctx, eventNIDs)
	if err != nil {
		return nil, err
	}

	result := make([]gomatrixserverlib.Event, len(stateEvents))
	for i := range stateEvents {
		result[i] = stateEvents[i].Event
	}
	return result, nil
}

// QueryMembershipForUser implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryMembershipForUser(
	ctx context.Context,
	request *api.QueryMembershipForUserRequest,
	response *api.QueryMembershipForUserResponse,
) error {
	roomNID, err := r.DB.RoomNID(ctx, request.RoomID)
	if err != nil {
		return err
	}

	membershipEventNID, stillInRoom, err := r.DB.GetMembership(ctx, roomNID, request.UserID)
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
	roomNID, err := r.DB.RoomNID(ctx, request.RoomID)
	if err != nil {
		return err
	}

	membershipEventNID, stillInRoom, err := r.DB.GetMembership(ctx, roomNID, request.Sender)
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
		eventNIDs, err = r.DB.GetMembershipEventNIDsForRoom(ctx, roomNID, request.JoinedOnly, false)
		if err != nil {
			return err
		}

		events, err = r.DB.Events(ctx, eventNIDs)
	} else {
		stateEntries, err = stateBeforeEvent(ctx, r.DB, membershipEventNID)
		if err != nil {
			logrus.WithField("membership_event_nid", membershipEventNID).WithError(err).Error("failed to load state before event")
			return err
		}
		events, err = getMembershipsAtState(ctx, r.DB, stateEntries, request.JoinedOnly)
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

func stateBeforeEvent(ctx context.Context, db storage.Database, eventNID types.EventNID) ([]types.StateEntry, error) {
	roomState := state.NewStateResolution(db)
	// Lookup the event NID
	eIDs, err := db.EventIDs(ctx, []types.EventNID{eventNID})
	if err != nil {
		return nil, err
	}
	eventIDs := []string{eIDs[eventNID]}

	prevState, err := db.StateAtEventIDs(ctx, eventIDs)
	if err != nil {
		return nil, err
	}

	// Fetch the state as it was when this event was fired
	return roomState.LoadCombinedStateAfterEvents(ctx, prevState)
}

// getMembershipsAtState filters the state events to
// only keep the "m.room.member" events with a "join" membership. These events are returned.
// Returns an error if there was an issue fetching the events.
func getMembershipsAtState(
	ctx context.Context, db storage.Database, stateEntries []types.StateEntry, joinedOnly bool,
) ([]types.Event, error) {

	var eventNIDs []types.EventNID
	for _, entry := range stateEntries {
		// Filter the events to retrieve to only keep the membership events
		if entry.EventTypeNID == types.MRoomMemberNID {
			eventNIDs = append(eventNIDs, entry.EventNID)
		}
	}

	// Get all of the events in this state
	stateEvents, err := db.Events(ctx, eventNIDs)
	if err != nil {
		return nil, err
	}

	if !joinedOnly {
		return stateEvents, nil
	}

	// Filter the events to only keep the "join" membership events
	var events []types.Event
	for _, event := range stateEvents {
		membership, err := event.Membership()
		if err != nil {
			return nil, err
		}

		if membership == gomatrixserverlib.Join {
			events = append(events, event)
		}
	}

	return events, nil
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
	isServerInRoom, err := r.isServerCurrentlyInRoom(ctx, request.ServerName, events[0].RoomID())
	if err != nil {
		return
	}
	response.AllowedToSeeEvent, err = r.checkServerAllowedToSeeEvent(
		ctx, request.EventID, request.ServerName, isServerInRoom,
	)
	return
}

func (r *RoomserverInternalAPI) checkServerAllowedToSeeEvent(
	ctx context.Context, eventID string, serverName gomatrixserverlib.ServerName, isServerInRoom bool,
) (bool, error) {
	roomState := state.NewStateResolution(r.DB)
	stateEntries, err := roomState.LoadStateAtEvent(ctx, eventID)
	if err != nil {
		return false, err
	}

	// TODO: We probably want to make it so that we don't have to pull
	// out all the state if possible.
	stateAtEvent, err := r.loadStateEvents(ctx, stateEntries)
	if err != nil {
		return false, err
	}

	return auth.IsServerAllowed(serverName, isServerInRoom, stateAtEvent), nil
}

// QueryMissingEvents implements api.RoomserverInternalAPI
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

	resultNIDs, err := r.scanEventTree(ctx, front, visited, request.Limit, request.ServerName)
	if err != nil {
		return err
	}

	loadedEvents, err := r.loadEvents(ctx, resultNIDs)
	if err != nil {
		return err
	}

	response.Events = make([]gomatrixserverlib.HeaderedEvent, 0, len(loadedEvents)-len(eventsToFilter))
	for _, event := range loadedEvents {
		if !eventsToFilter[event.EventID()] {
			roomVersion, verr := r.DB.GetRoomVersionForRoom(ctx, event.RoomID())
			if verr != nil {
				return verr
			}

			response.Events = append(response.Events, event.Headered(roomVersion))
		}
	}

	return err
}

// PerformBackfill implements api.RoomServerQueryAPI
func (r *RoomserverInternalAPI) PerformBackfill(
	ctx context.Context,
	request *api.PerformBackfillRequest,
	response *api.PerformBackfillResponse,
) error {
	// if we are requesting the backfill then we need to do a federation hit
	// TODO: we could be more sensible and fetch as many events we already have then request the rest
	//       which is what the syncapi does already.
	if request.ServerName == r.ServerName {
		return r.backfillViaFederation(ctx, request, response)
	}
	// someone else is requesting the backfill, try to service their request.
	var err error
	var front []string

	// The limit defines the maximum number of events to retrieve, so it also
	// defines the highest number of elements in the map below.
	visited := make(map[string]bool, request.Limit)

	// this will include these events which is what we want
	front = request.PrevEventIDs()

	// Scan the event tree for events to send back.
	resultNIDs, err := r.scanEventTree(ctx, front, visited, request.Limit, request.ServerName)
	if err != nil {
		return err
	}

	// Retrieve events from the list that was filled previously.
	var loadedEvents []gomatrixserverlib.Event
	loadedEvents, err = r.loadEvents(ctx, resultNIDs)
	if err != nil {
		return err
	}

	for _, event := range loadedEvents {
		roomVersion, verr := r.DB.GetRoomVersionForRoom(ctx, event.RoomID())
		if verr != nil {
			return verr
		}

		response.Events = append(response.Events, event.Headered(roomVersion))
	}

	return err
}

func (r *RoomserverInternalAPI) backfillViaFederation(ctx context.Context, req *api.PerformBackfillRequest, res *api.PerformBackfillResponse) error {
	roomVer, err := r.DB.GetRoomVersionForRoom(ctx, req.RoomID)
	if err != nil {
		return fmt.Errorf("backfillViaFederation: unknown room version for room %s : %w", req.RoomID, err)
	}
	requester := newBackfillRequester(r.DB, r.FedClient, r.ServerName, req.BackwardsExtremities)
	// Request 100 items regardless of what the query asks for.
	// We don't want to go much higher than this.
	// We can't honour exactly the limit as some sytests rely on requesting more for tests to pass
	// (so we don't need to hit /state_ids which the test has no listener for)
	// Specifically the test "Outbound federation can backfill events"
	events, err := gomatrixserverlib.RequestBackfill(
		ctx, requester,
		r.KeyRing, req.RoomID, roomVer, req.PrevEventIDs(), 100)
	if err != nil {
		return err
	}
	logrus.WithField("room_id", req.RoomID).Infof("backfilled %d events", len(events))

	// persist these new events - auth checks have already been done
	roomNID, backfilledEventMap := persistEvents(ctx, r.DB, events)
	if err != nil {
		return err
	}

	for _, ev := range backfilledEventMap {
		// now add state for these events
		stateIDs, ok := requester.eventIDToBeforeStateIDs[ev.EventID()]
		if !ok {
			// this should be impossible as all events returned must have pass Step 5 of the PDU checks
			// which requires a list of state IDs.
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("backfillViaFederation: failed to find state IDs for event which passed auth checks")
			continue
		}
		var entries []types.StateEntry
		if entries, err = r.DB.StateEntriesForEventIDs(ctx, stateIDs); err != nil {
			// attempt to fetch the missing events
			r.fetchAndStoreMissingEvents(ctx, roomVer, requester, stateIDs)
			// try again
			entries, err = r.DB.StateEntriesForEventIDs(ctx, stateIDs)
			if err != nil {
				logrus.WithError(err).WithField("event_id", ev.EventID()).Error("backfillViaFederation: failed to get state entries for event")
				return err
			}
		}

		var beforeStateSnapshotNID types.StateSnapshotNID
		if beforeStateSnapshotNID, err = r.DB.AddState(ctx, roomNID, nil, entries); err != nil {
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("backfillViaFederation: failed to persist state entries to get snapshot nid")
			return err
		}
		if err = r.DB.SetState(ctx, ev.EventNID, beforeStateSnapshotNID); err != nil {
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("backfillViaFederation: failed to persist snapshot nid")
		}
	}

	// TODO: update backwards extremities, as that should be moved from syncapi to roomserver at some point.

	res.Events = events
	return nil
}

func (r *RoomserverInternalAPI) isServerCurrentlyInRoom(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID string) (bool, error) {
	roomNID, err := r.DB.RoomNID(ctx, roomID)
	if err != nil {
		return false, err
	}

	eventNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, roomNID, true, false)
	if err != nil {
		return false, err
	}

	events, err := r.DB.Events(ctx, eventNIDs)
	if err != nil {
		return false, err
	}
	gmslEvents := make([]gomatrixserverlib.Event, len(events))
	for i := range events {
		gmslEvents[i] = events[i].Event
	}
	return auth.IsAnyUserOnServerWithMembership(serverName, gmslEvents, gomatrixserverlib.Join), nil
}

// fetchAndStoreMissingEvents does a best-effort fetch and store of missing events specified in stateIDs. Returns no error as it is just
// best effort.
func (r *RoomserverInternalAPI) fetchAndStoreMissingEvents(ctx context.Context, roomVer gomatrixserverlib.RoomVersion,
	backfillRequester *backfillRequester, stateIDs []string) {

	servers := backfillRequester.servers

	// work out which are missing
	nidMap, err := r.DB.EventNIDs(ctx, stateIDs)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Warn("cannot query missing events")
		return
	}
	missingMap := make(map[string]*gomatrixserverlib.HeaderedEvent) // id -> event
	for _, id := range stateIDs {
		if _, ok := nidMap[id]; !ok {
			missingMap[id] = nil
		}
	}
	util.GetLogger(ctx).Infof("Fetching %d missing state events (from %d possible servers)", len(missingMap), len(servers))

	// fetch the events from federation. Loop the servers first so if we find one that works we stick with them
	for _, srv := range servers {
		for id, ev := range missingMap {
			if ev != nil {
				continue // already found
			}
			logger := util.GetLogger(ctx).WithField("server", srv).WithField("event_id", id)
			res, err := r.FedClient.GetEvent(ctx, srv, id)
			if err != nil {
				logger.WithError(err).Warn("failed to get event from server")
				continue
			}
			loader := gomatrixserverlib.NewEventsLoader(roomVer, r.KeyRing, backfillRequester, backfillRequester.ProvideEvents, false)
			result, err := loader.LoadAndVerify(ctx, res.PDUs, gomatrixserverlib.TopologicalOrderByPrevEvents)
			if err != nil {
				logger.WithError(err).Warn("failed to load and verify event")
				continue
			}
			logger.Infof("returned %d PDUs which made events %+v", len(res.PDUs), result)
			for _, res := range result {
				if res.Error != nil {
					logger.WithError(err).Warn("event failed PDU checks")
					continue
				}
				missingMap[id] = res.Event
			}
		}
	}

	var newEvents []gomatrixserverlib.HeaderedEvent
	for _, ev := range missingMap {
		if ev != nil {
			newEvents = append(newEvents, *ev)
		}
	}
	util.GetLogger(ctx).Infof("Persisting %d new events", len(newEvents))
	persistEvents(ctx, r.DB, newEvents)
}

// TODO: Remove this when we have tests to assert correctness of this function
// nolint:gocyclo
func (r *RoomserverInternalAPI) scanEventTree(
	ctx context.Context, front []string, visited map[string]bool, limit int,
	serverName gomatrixserverlib.ServerName,
) ([]types.EventNID, error) {
	var resultNIDs []types.EventNID
	var err error
	var allowed bool
	var events []types.Event
	var next []string
	var pre string

	// TODO: add tests for this function to ensure it meets the contract that callers expect (and doc what that is supposed to be)
	// Currently, callers like PerformBackfill will call scanEventTree with a pre-populated `visited` map, assuming that by doing
	// so means that the events in that map will NOT be returned from this function. That is not currently true, resulting in
	// duplicate events being sent in response to /backfill requests.
	initialIgnoreList := make(map[string]bool, len(visited))
	for k, v := range visited {
		initialIgnoreList[k] = v
	}

	resultNIDs = make([]types.EventNID, 0, limit)

	var checkedServerInRoom bool
	var isServerInRoom bool

	// Loop through the event IDs to retrieve the requested events and go
	// through the whole tree (up to the provided limit) using the events'
	// "prev_event" key.
BFSLoop:
	for len(front) > 0 {
		// Prevent unnecessary allocations: reset the slice only when not empty.
		if len(next) > 0 {
			next = make([]string, 0)
		}
		// Retrieve the events to process from the database.
		events, err = r.DB.EventsFromIDs(ctx, front)
		if err != nil {
			return resultNIDs, err
		}

		if !checkedServerInRoom && len(events) > 0 {
			// It's nasty that we have to extract the room ID from an event, but many federation requests
			// only talk in event IDs, no room IDs at all (!!!)
			ev := events[0]
			isServerInRoom, err = r.isServerCurrentlyInRoom(ctx, serverName, ev.RoomID())
			if err != nil {
				util.GetLogger(ctx).WithError(err).Error("Failed to check if server is currently in room, assuming not.")
			}
			checkedServerInRoom = true
		}

		for _, ev := range events {
			// Break out of the loop if the provided limit is reached.
			if len(resultNIDs) == limit {
				break BFSLoop
			}

			if !initialIgnoreList[ev.EventID()] {
				// Update the list of events to retrieve.
				resultNIDs = append(resultNIDs, ev.EventNID)
			}
			// Loop through the event's parents.
			for _, pre = range ev.PrevEventIDs() {
				// Only add an event to the list of next events to process if it
				// hasn't been seen before.
				if !visited[pre] {
					visited[pre] = true
					allowed, err = r.checkServerAllowedToSeeEvent(ctx, pre, serverName, isServerInRoom)
					if err != nil {
						util.GetLogger(ctx).WithField("server", serverName).WithField("event_id", pre).WithError(err).Error(
							"Error checking if allowed to see event",
						)
						return resultNIDs, err
					}

					// If the event hasn't been seen before and the HS
					// requesting to retrieve it is allowed to do so, add it to
					// the list of events to retrieve.
					if allowed {
						next = append(next, pre)
					} else {
						util.GetLogger(ctx).WithField("server", serverName).WithField("event_id", pre).Info("Not allowed to see event")
					}
				}
			}
		}
		// Repeat the same process with the parent events we just processed.
		front = next
	}

	return resultNIDs, err
}

// QueryStateAndAuthChain implements api.RoomserverInternalAPI
func (r *RoomserverInternalAPI) QueryStateAndAuthChain(
	ctx context.Context,
	request *api.QueryStateAndAuthChainRequest,
	response *api.QueryStateAndAuthChainResponse,
) error {
	roomNID, err := r.DB.RoomNIDExcludingStubs(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true

	roomVersion, err := r.DB.GetRoomVersionForRoom(ctx, request.RoomID)
	if err != nil {
		return err
	}
	response.RoomVersion = roomVersion

	stateEvents, err := r.loadStateAtEventIDs(ctx, request.PrevEventIDs)
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
			roomVersion, stateEvents, authEvents,
		); err != nil {
			return err
		}
	}

	for _, event := range stateEvents {
		response.StateEvents = append(response.StateEvents, event.Headered(roomVersion))
	}

	for _, event := range authEvents {
		response.AuthChainEvents = append(response.AuthChainEvents, event.Headered(roomVersion))
	}

	return err
}

func (r *RoomserverInternalAPI) loadStateAtEventIDs(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.Event, error) {
	roomState := state.NewStateResolution(r.DB)
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

	return r.loadStateEvents(ctx, stateEntries)
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

func persistEvents(ctx context.Context, db storage.Database, events []gomatrixserverlib.HeaderedEvent) (types.RoomNID, map[string]types.Event) {
	var roomNID types.RoomNID
	backfilledEventMap := make(map[string]types.Event)
	for j, ev := range events {
		nidMap, err := db.EventNIDs(ctx, ev.AuthEventIDs())
		if err != nil { // this shouldn't happen as RequestBackfill already found them
			logrus.WithError(err).WithField("auth_events", ev.AuthEventIDs()).Error("Failed to find one or more auth events")
			continue
		}
		authNids := make([]types.EventNID, len(nidMap))
		i := 0
		for _, nid := range nidMap {
			authNids[i] = nid
			i++
		}
		var stateAtEvent types.StateAtEvent
		var redactedEventID string
		var redactionEvent *gomatrixserverlib.Event
		roomNID, stateAtEvent, redactionEvent, redactedEventID, err = db.StoreEvent(ctx, ev.Unwrap(), nil, authNids)
		if err != nil {
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("Failed to persist event")
			continue
		}
		// If storing this event results in it being redacted, then do so.
		// It's also possible for this event to be a redaction which results in another event being
		// redacted, which we don't care about since we aren't returning it in this backfill.
		if redactedEventID == ev.EventID() {
			eventToRedact := ev.Unwrap()
			redactedEvent, err := eventutil.RedactEvent(redactionEvent, &eventToRedact)
			if err != nil {
				logrus.WithError(err).WithField("event_id", ev.EventID()).Error("Failed to redact event")
				continue
			}
			ev = redactedEvent.Headered(ev.RoomVersion)
			events[j] = ev
		}
		backfilledEventMap[ev.EventID()] = types.Event{
			EventNID: stateAtEvent.StateEntry.EventNID,
			Event:    ev.Unwrap(),
		}
	}
	return roomNID, backfilledEventMap
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

	roomVersion, err := r.DB.GetRoomVersionForRoom(ctx, request.RoomID)
	if err != nil {
		return err
	}
	response.RoomVersion = roomVersion
	r.Cache.StoreRoomVersion(request.RoomID, response.RoomVersion)
	return nil
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
