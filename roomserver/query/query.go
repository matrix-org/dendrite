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

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/caching"
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

// RoomserverQueryAPI is an implementation of api.RoomserverQueryAPI
type RoomserverQueryAPI struct {
	DB             storage.Database
	ImmutableCache caching.ImmutableCache
	ServerName     gomatrixserverlib.ServerName
	KeyRing        gomatrixserverlib.JSONVerifier
	FedClient      *gomatrixserverlib.FederationClient
}

// QueryLatestEventsAndState implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryLatestEventsAndState(
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

	response.QueryLatestEventsAndStateRequest = *request
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

// QueryStateAfterEvents implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryStateAfterEvents(
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

	response.QueryStateAfterEventsRequest = *request
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

// QueryEventsByID implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryEventsByID(
	ctx context.Context,
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	response.QueryEventsByIDRequest = *request

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

func (r *RoomserverQueryAPI) loadStateEvents(
	ctx context.Context, stateEntries []types.StateEntry,
) ([]gomatrixserverlib.Event, error) {
	eventNIDs := make([]types.EventNID, len(stateEntries))
	for i := range stateEntries {
		eventNIDs[i] = stateEntries[i].EventNID
	}
	return r.loadEvents(ctx, eventNIDs)
}

func (r *RoomserverQueryAPI) loadEvents(
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

// QueryMembershipForUser implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryMembershipForUser(
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
	eventIDMap, err := r.DB.EventIDs(ctx, []types.EventNID{membershipEventNID})
	if err != nil {
		return err
	}

	response.EventID = eventIDMap[membershipEventNID]
	return nil
}

// QueryMembershipsForRoom implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryMembershipsForRoom(
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
	if stillInRoom {
		var eventNIDs []types.EventNID
		eventNIDs, err = r.DB.GetMembershipEventNIDsForRoom(ctx, roomNID, request.JoinedOnly)
		if err != nil {
			return err
		}

		events, err = r.DB.Events(ctx, eventNIDs)
	} else {
		events, err = getMembershipsBeforeEventNID(ctx, r.DB, membershipEventNID, request.JoinedOnly)
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

// getMembershipsBeforeEventNID takes the numeric ID of an event and fetches the state
// of the event's room as it was when this event was fired, then filters the state events to
// only keep the "m.room.member" events with a "join" membership. These events are returned.
// Returns an error if there was an issue fetching the events.
func getMembershipsBeforeEventNID(
	ctx context.Context, db storage.Database, eventNID types.EventNID, joinedOnly bool,
) ([]types.Event, error) {
	roomState := state.NewStateResolution(db)
	events := []types.Event{}
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
	stateEntries, err := roomState.LoadCombinedStateAfterEvents(ctx, prevState)
	if err != nil {
		return nil, err
	}

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

// QueryInvitesForUser implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryInvitesForUser(
	ctx context.Context,
	request *api.QueryInvitesForUserRequest,
	response *api.QueryInvitesForUserResponse,
) error {
	roomNID, err := r.DB.RoomNID(ctx, request.RoomID)
	if err != nil {
		return err
	}

	targetUserNIDs, err := r.DB.EventStateKeyNIDs(ctx, []string{request.TargetUserID})
	if err != nil {
		return err
	}
	targetUserNID := targetUserNIDs[request.TargetUserID]

	senderUserNIDs, err := r.DB.GetInvitesForUser(ctx, roomNID, targetUserNID)
	if err != nil {
		return err
	}

	senderUserIDs, err := r.DB.EventStateKeys(ctx, senderUserNIDs)
	if err != nil {
		return err
	}

	for _, senderUserID := range senderUserIDs {
		response.InviteSenderUserIDs = append(response.InviteSenderUserIDs, senderUserID)
	}

	return nil
}

// QueryServerAllowedToSeeEvent implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryServerAllowedToSeeEvent(
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

func (r *RoomserverQueryAPI) checkServerAllowedToSeeEvent(
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

// QueryMissingEvents implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryMissingEvents(
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

// QueryBackfill implements api.RoomServerQueryAPI
func (r *RoomserverQueryAPI) QueryBackfill(
	ctx context.Context,
	request *api.QueryBackfillRequest,
	response *api.QueryBackfillResponse,
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

	// The provided event IDs have already been seen by the request's emitter,
	// and will be retrieved anyway, so there's no need to care about them if
	// they appear in our exploration of the event tree.
	for _, id := range request.EarliestEventsIDs {
		visited[id] = true
	}

	front = request.EarliestEventsIDs

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

func (r *RoomserverQueryAPI) backfillViaFederation(ctx context.Context, req *api.QueryBackfillRequest, res *api.QueryBackfillResponse) error {
	roomVer, err := r.DB.GetRoomVersionForRoom(ctx, req.RoomID)
	if err != nil {
		return fmt.Errorf("backfillViaFederation: unknown room version for room %s : %w", req.RoomID, err)
	}
	requester := newBackfillRequester(r.DB, r.FedClient, r.ServerName)
	events, err := gomatrixserverlib.RequestBackfill(
		ctx, requester,
		r.KeyRing, req.RoomID, roomVer, req.EarliestEventsIDs, req.Limit)
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
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("Failed to find state IDs for event which passed auth checks")
			continue
		}
		var entries []types.StateEntry
		if entries, err = r.DB.StateEntriesForEventIDs(ctx, stateIDs); err != nil {
			return err
		}

		var beforeStateSnapshotNID types.StateSnapshotNID
		if beforeStateSnapshotNID, err = r.DB.AddState(ctx, roomNID, nil, entries); err != nil {
			return err
		}
		if err = r.DB.SetState(ctx, ev.EventNID, beforeStateSnapshotNID); err != nil {
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("Failed to set state before event")
		}
	}

	// TODO: update backwards extremities, as that should be moved from syncapi to roomserver at some point.

	res.Events = events
	return nil
}

func (r *RoomserverQueryAPI) isServerCurrentlyInRoom(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID string) (bool, error) {
	roomNID, err := r.DB.RoomNID(ctx, roomID)
	if err != nil {
		return false, err
	}

	eventNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, roomNID, true)
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

// TODO: Remove this when we have tests to assert correctness of this function
// nolint:gocyclo
func (r *RoomserverQueryAPI) scanEventTree(
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
	// Currently, callers like QueryBackfill will call scanEventTree with a pre-populated `visited` map, assuming that by doing
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

// QueryStateAndAuthChain implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryStateAndAuthChain(
	ctx context.Context,
	request *api.QueryStateAndAuthChainRequest,
	response *api.QueryStateAndAuthChainResponse,
) error {
	response.QueryStateAndAuthChainRequest = *request
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

func (r *RoomserverQueryAPI) loadStateAtEventIDs(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.Event, error) {
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
	for _, ev := range events {
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
		roomNID, stateAtEvent, err = db.StoreEvent(ctx, ev.Unwrap(), nil, authNids)
		if err != nil {
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("Failed to store backfilled event")
			continue
		}
		backfilledEventMap[ev.EventID()] = types.Event{
			EventNID: stateAtEvent.StateEntry.EventNID,
			Event:    ev.Unwrap(),
		}
	}
	return roomNID, backfilledEventMap
}

// QueryRoomVersionCapabilities implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryRoomVersionCapabilities(
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

// QueryRoomVersionCapabilities implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryRoomVersionForRoom(
	ctx context.Context,
	request *api.QueryRoomVersionForRoomRequest,
	response *api.QueryRoomVersionForRoomResponse,
) error {
	if roomVersion, ok := r.ImmutableCache.GetRoomVersion(request.RoomID); ok {
		response.RoomVersion = roomVersion
		return nil
	}

	roomVersion, err := r.DB.GetRoomVersionForRoom(ctx, request.RoomID)
	if err != nil {
		return err
	}
	response.RoomVersion = roomVersion
	r.ImmutableCache.StoreRoomVersion(request.RoomID, response.RoomVersion)
	return nil
}

// SetupHTTP adds the RoomserverQueryAPI handlers to the http.ServeMux.
// nolint: gocyclo
func (r *RoomserverQueryAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.RoomserverQueryLatestEventsAndStatePath,
		common.MakeInternalAPI("queryLatestEventsAndState", func(req *http.Request) util.JSONResponse {
			var request api.QueryLatestEventsAndStateRequest
			var response api.QueryLatestEventsAndStateResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryLatestEventsAndState(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryStateAfterEventsPath,
		common.MakeInternalAPI("queryStateAfterEvents", func(req *http.Request) util.JSONResponse {
			var request api.QueryStateAfterEventsRequest
			var response api.QueryStateAfterEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryStateAfterEvents(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryEventsByIDPath,
		common.MakeInternalAPI("queryEventsByID", func(req *http.Request) util.JSONResponse {
			var request api.QueryEventsByIDRequest
			var response api.QueryEventsByIDResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryEventsByID(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryMembershipForUserPath,
		common.MakeInternalAPI("QueryMembershipForUser", func(req *http.Request) util.JSONResponse {
			var request api.QueryMembershipForUserRequest
			var response api.QueryMembershipForUserResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryMembershipForUser(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryMembershipsForRoomPath,
		common.MakeInternalAPI("queryMembershipsForRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryMembershipsForRoomRequest
			var response api.QueryMembershipsForRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryMembershipsForRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryInvitesForUserPath,
		common.MakeInternalAPI("queryInvitesForUser", func(req *http.Request) util.JSONResponse {
			var request api.QueryInvitesForUserRequest
			var response api.QueryInvitesForUserResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryInvitesForUser(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryServerAllowedToSeeEventPath,
		common.MakeInternalAPI("queryServerAllowedToSeeEvent", func(req *http.Request) util.JSONResponse {
			var request api.QueryServerAllowedToSeeEventRequest
			var response api.QueryServerAllowedToSeeEventResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryServerAllowedToSeeEvent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryMissingEventsPath,
		common.MakeInternalAPI("queryMissingEvents", func(req *http.Request) util.JSONResponse {
			var request api.QueryMissingEventsRequest
			var response api.QueryMissingEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryMissingEvents(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryStateAndAuthChainPath,
		common.MakeInternalAPI("queryStateAndAuthChain", func(req *http.Request) util.JSONResponse {
			var request api.QueryStateAndAuthChainRequest
			var response api.QueryStateAndAuthChainResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryStateAndAuthChain(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryBackfillPath,
		common.MakeInternalAPI("QueryBackfill", func(req *http.Request) util.JSONResponse {
			var request api.QueryBackfillRequest
			var response api.QueryBackfillResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryBackfill(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryRoomVersionCapabilitiesPath,
		common.MakeInternalAPI("QueryRoomVersionCapabilities", func(req *http.Request) util.JSONResponse {
			var request api.QueryRoomVersionCapabilitiesRequest
			var response api.QueryRoomVersionCapabilitiesResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryRoomVersionCapabilities(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryRoomVersionForRoomPath,
		common.MakeInternalAPI("QueryRoomVersionForRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryRoomVersionForRoomRequest
			var response api.QueryRoomVersionForRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryRoomVersionForRoom(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
