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

package perform

import (
	"context"
	"fmt"

	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/auth"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// the max number of servers to backfill from per request. If this is too low we may fail to backfill when
// we could've from another server. If this is too high we may take far too long to successfully backfill
// as we try dead servers.
const maxBackfillServers = 5

type Backfiller struct {
	ServerName gomatrixserverlib.ServerName
	DB         storage.Database
	FSAPI      federationAPI.FederationInternalAPI
	KeyRing    gomatrixserverlib.JSONVerifier

	// The servers which should be preferred above other servers when backfilling
	PreferServers []gomatrixserverlib.ServerName
}

// PerformBackfill implements api.RoomServerQueryAPI
func (r *Backfiller) PerformBackfill(
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

	info, err := r.DB.RoomInfo(ctx, request.RoomID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub {
		return fmt.Errorf("PerformBackfill: missing room info for room %s", request.RoomID)
	}

	// Scan the event tree for events to send back.
	resultNIDs, err := helpers.ScanEventTree(ctx, r.DB, *info, front, visited, request.Limit, request.ServerName)
	if err != nil {
		return err
	}

	// Retrieve events from the list that was filled previously.
	var loadedEvents []*gomatrixserverlib.Event
	loadedEvents, err = helpers.LoadEvents(ctx, r.DB, resultNIDs)
	if err != nil {
		return err
	}

	for _, event := range loadedEvents {
		response.Events = append(response.Events, event.Headered(info.RoomVersion))
	}

	return err
}

func (r *Backfiller) backfillViaFederation(ctx context.Context, req *api.PerformBackfillRequest, res *api.PerformBackfillResponse) error {
	info, err := r.DB.RoomInfo(ctx, req.RoomID)
	if err != nil {
		return err
	}
	if info == nil || info.IsStub {
		return fmt.Errorf("backfillViaFederation: missing room info for room %s", req.RoomID)
	}
	requester := newBackfillRequester(r.DB, r.FSAPI, r.ServerName, req.BackwardsExtremities, r.PreferServers)
	// Request 100 items regardless of what the query asks for.
	// We don't want to go much higher than this.
	// We can't honour exactly the limit as some sytests rely on requesting more for tests to pass
	// (so we don't need to hit /state_ids which the test has no listener for)
	// Specifically the test "Outbound federation can backfill events"
	events, err := gomatrixserverlib.RequestBackfill(
		ctx, requester,
		r.KeyRing, req.RoomID, info.RoomVersion, req.PrevEventIDs(), 100)
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
			r.fetchAndStoreMissingEvents(ctx, info.RoomVersion, requester, stateIDs)
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

// fetchAndStoreMissingEvents does a best-effort fetch and store of missing events specified in stateIDs. Returns no error as it is just
// best effort.
func (r *Backfiller) fetchAndStoreMissingEvents(ctx context.Context, roomVer gomatrixserverlib.RoomVersion,
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
			res, err := r.FSAPI.GetEvent(ctx, srv, id)
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
					logger.WithError(res.Error).Warn("event failed PDU checks")
					continue
				}
				missingMap[id] = res.Event
			}
		}
	}

	var newEvents []*gomatrixserverlib.HeaderedEvent
	for _, ev := range missingMap {
		if ev != nil {
			newEvents = append(newEvents, ev)
		}
	}
	util.GetLogger(ctx).Infof("Persisting %d new events", len(newEvents))
	persistEvents(ctx, r.DB, newEvents)
}

// backfillRequester implements gomatrixserverlib.BackfillRequester
type backfillRequester struct {
	db           storage.Database
	fsAPI        federationAPI.FederationInternalAPI
	thisServer   gomatrixserverlib.ServerName
	preferServer map[gomatrixserverlib.ServerName]bool
	bwExtrems    map[string][]string

	// per-request state
	servers                 []gomatrixserverlib.ServerName
	eventIDToBeforeStateIDs map[string][]string
	eventIDMap              map[string]*gomatrixserverlib.Event
}

func newBackfillRequester(
	db storage.Database, fsAPI federationAPI.FederationInternalAPI, thisServer gomatrixserverlib.ServerName,
	bwExtrems map[string][]string, preferServers []gomatrixserverlib.ServerName,
) *backfillRequester {
	preferServer := make(map[gomatrixserverlib.ServerName]bool)
	for _, p := range preferServers {
		preferServer[p] = true
	}
	return &backfillRequester{
		db:                      db,
		fsAPI:                   fsAPI,
		thisServer:              thisServer,
		eventIDToBeforeStateIDs: make(map[string][]string),
		eventIDMap:              make(map[string]*gomatrixserverlib.Event),
		bwExtrems:               bwExtrems,
		preferServer:            preferServer,
	}
}

func (b *backfillRequester) StateIDsBeforeEvent(ctx context.Context, targetEvent *gomatrixserverlib.HeaderedEvent) ([]string, error) {
	b.eventIDMap[targetEvent.EventID()] = targetEvent.Unwrap()
	if ids, ok := b.eventIDToBeforeStateIDs[targetEvent.EventID()]; ok {
		return ids, nil
	}
	if len(targetEvent.PrevEventIDs()) == 0 && targetEvent.Type() == "m.room.create" && targetEvent.StateKeyEquals("") {
		util.GetLogger(ctx).WithField("room_id", targetEvent.RoomID()).Info("Backfilled to the beginning of the room")
		b.eventIDToBeforeStateIDs[targetEvent.EventID()] = []string{}
		return nil, nil
	}
	// if we have exactly 1 prev event and we know the state of the room at that prev event, then just roll forward the prev event.
	// Else, we have to hit /state_ids because either we don't know the state at all at this event (new backwards extremity) or
	// we don't know the result of state res to merge forks (2 or more prev_events)
	if len(targetEvent.PrevEventIDs()) == 1 {
		prevEventID := targetEvent.PrevEventIDs()[0]
		prevEvent, ok := b.eventIDMap[prevEventID]
		if !ok {
			goto FederationHit
		}
		prevEventStateIDs, ok := b.eventIDToBeforeStateIDs[prevEventID]
		if !ok {
			goto FederationHit
		}
		newStateIDs := b.calculateNewStateIDs(targetEvent.Unwrap(), prevEvent, prevEventStateIDs)
		if newStateIDs != nil {
			b.eventIDToBeforeStateIDs[targetEvent.EventID()] = newStateIDs
			return newStateIDs, nil
		}
		// else we failed to calculate the new state, so fallthrough
	}

FederationHit:
	var lastErr error
	logrus.WithField("event_id", targetEvent.EventID()).Info("Requesting /state_ids at event")
	for _, srv := range b.servers { // hit any valid server
		c := gomatrixserverlib.FederatedStateProvider{
			FedClient:          b.fsAPI,
			RememberAuthEvents: false,
			Server:             srv,
		}
		res, err := c.StateIDsBeforeEvent(ctx, targetEvent)
		if err != nil {
			lastErr = err
			continue
		}
		b.eventIDToBeforeStateIDs[targetEvent.EventID()] = res
		return res, nil
	}
	return nil, lastErr
}

func (b *backfillRequester) calculateNewStateIDs(targetEvent, prevEvent *gomatrixserverlib.Event, prevEventStateIDs []string) []string {
	newStateIDs := prevEventStateIDs[:]
	if prevEvent.StateKey() == nil {
		// state is the same as the previous event
		b.eventIDToBeforeStateIDs[targetEvent.EventID()] = newStateIDs
		return newStateIDs
	}

	missingState := false // true if we are missing the info for a state event ID
	foundEvent := false   // true if we found a (type, state_key) match
	// find which state ID to replace, if any
	for i, id := range newStateIDs {
		ev, ok := b.eventIDMap[id]
		if !ok {
			missingState = true
			continue
		}
		// The state IDs BEFORE the target event are the state IDs BEFORE the prev_event PLUS the prev_event itself
		if ev.Type() == prevEvent.Type() && ev.StateKeyEquals(*prevEvent.StateKey()) {
			newStateIDs[i] = prevEvent.EventID()
			foundEvent = true
			break
		}
	}
	if !foundEvent && !missingState {
		// we can be certain that this is new state
		newStateIDs = append(newStateIDs, prevEvent.EventID())
		foundEvent = true
	}

	if foundEvent {
		b.eventIDToBeforeStateIDs[targetEvent.EventID()] = newStateIDs
		return newStateIDs
	}
	return nil
}

func (b *backfillRequester) StateBeforeEvent(ctx context.Context, roomVer gomatrixserverlib.RoomVersion,
	event *gomatrixserverlib.HeaderedEvent, eventIDs []string) (map[string]*gomatrixserverlib.Event, error) {

	// try to fetch the events from the database first
	events, err := b.ProvideEvents(roomVer, eventIDs)
	if err != nil {
		// non-fatal, fallthrough
		logrus.WithError(err).Info("Failed to fetch events")
	} else {
		logrus.Infof("Fetched %d/%d events from the database", len(events), len(eventIDs))
		if len(events) == len(eventIDs) {
			result := make(map[string]*gomatrixserverlib.Event)
			for i := range events {
				result[events[i].EventID()] = events[i]
				b.eventIDMap[events[i].EventID()] = events[i]
			}
			return result, nil
		}
	}

	c := gomatrixserverlib.FederatedStateProvider{
		FedClient:          b.fsAPI,
		RememberAuthEvents: false,
		Server:             b.servers[0],
	}
	result, err := c.StateBeforeEvent(ctx, roomVer, event, eventIDs)
	if err != nil {
		return nil, err
	}
	for eventID, ev := range result {
		b.eventIDMap[eventID] = ev
	}
	return result, nil
}

// ServersAtEvent is called when trying to determine which server to request from.
// It returns a list of servers which can be queried for backfill requests. These servers
// will be servers that are in the room already. The entries at the beginning are preferred servers
// and will be tried first. An empty list will fail the request.
func (b *backfillRequester) ServersAtEvent(ctx context.Context, roomID, eventID string) []gomatrixserverlib.ServerName {
	// eventID will be a prev_event ID of a backwards extremity, meaning we will not have a database entry for it. Instead, use
	// its successor, so look it up.
	successor := ""
FindSuccessor:
	for sucID, prevEventIDs := range b.bwExtrems {
		for _, pe := range prevEventIDs {
			if pe == eventID {
				successor = sucID
				break FindSuccessor
			}
		}
	}
	if successor == "" {
		logrus.WithField("event_id", eventID).Error("ServersAtEvent: failed to find successor of this event to determine room state")
		return nil
	}
	eventID = successor

	// getMembershipsBeforeEventNID requires a NID, so retrieving the NID for
	// the event is necessary.
	NIDs, err := b.db.EventNIDs(ctx, []string{eventID})
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to get event NID for event")
		return nil
	}

	info, err := b.db.RoomInfo(ctx, roomID)
	if err != nil {
		logrus.WithError(err).WithField("room_id", roomID).Error("ServersAtEvent: failed to get RoomInfo for room")
		return nil
	}
	if info == nil || info.IsStub {
		logrus.WithField("room_id", roomID).Error("ServersAtEvent: failed to get RoomInfo for room, room is missing")
		return nil
	}

	stateEntries, err := helpers.StateBeforeEvent(ctx, b.db, *info, NIDs[eventID])
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to load state before event")
		return nil
	}

	// possibly return all joined servers depending on history visiblity
	memberEventsFromVis, err := joinEventsFromHistoryVisibility(ctx, b.db, roomID, stateEntries, b.thisServer)
	if err != nil {
		logrus.WithError(err).Error("ServersAtEvent: failed calculate servers from history visibility rules")
		return nil
	}
	logrus.Infof("ServersAtEvent including %d current events from history visibility", len(memberEventsFromVis))

	// Retrieve all "m.room.member" state events of "join" membership, which
	// contains the list of users in the room before the event, therefore all
	// the servers in it at that moment.
	memberEvents, err := helpers.GetMembershipsAtState(ctx, b.db, stateEntries, true)
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to get memberships before event")
		return nil
	}
	memberEvents = append(memberEvents, memberEventsFromVis...)

	// Store the server names in a temporary map to avoid duplicates.
	serverSet := make(map[gomatrixserverlib.ServerName]bool)
	for _, event := range memberEvents {
		serverSet[event.Origin()] = true
	}
	var servers []gomatrixserverlib.ServerName
	for server := range serverSet {
		if server == b.thisServer {
			continue
		}
		if b.preferServer[server] { // insert at the front
			servers = append([]gomatrixserverlib.ServerName{server}, servers...)
		} else { // insert at the back
			servers = append(servers, server)
		}
	}
	if len(servers) > maxBackfillServers {
		servers = servers[:maxBackfillServers]
	}

	b.servers = servers
	return servers
}

// Backfill performs a backfill request to the given server.
// https://matrix.org/docs/spec/server_server/latest#get-matrix-federation-v1-backfill-roomid
func (b *backfillRequester) Backfill(ctx context.Context, server gomatrixserverlib.ServerName, roomID string,
	limit int, fromEventIDs []string) (gomatrixserverlib.Transaction, error) {

	tx, err := b.fsAPI.Backfill(ctx, server, roomID, limit, fromEventIDs)
	return tx, err
}

func (b *backfillRequester) ProvideEvents(roomVer gomatrixserverlib.RoomVersion, eventIDs []string) ([]*gomatrixserverlib.Event, error) {
	ctx := context.Background()
	nidMap, err := b.db.EventNIDs(ctx, eventIDs)
	if err != nil {
		logrus.WithError(err).WithField("event_ids", eventIDs).Error("Failed to find events")
		return nil, err
	}
	eventNIDs := make([]types.EventNID, len(nidMap))
	i := 0
	for _, nid := range nidMap {
		eventNIDs[i] = nid
		i++
	}
	eventsWithNids, err := b.db.Events(ctx, eventNIDs)
	if err != nil {
		logrus.WithError(err).WithField("event_nids", eventNIDs).Error("Failed to load events")
		return nil, err
	}
	events := make([]*gomatrixserverlib.Event, len(eventsWithNids))
	for i := range eventsWithNids {
		events[i] = eventsWithNids[i].Event
	}
	return events, nil
}

// joinEventsFromHistoryVisibility returns all CURRENTLY joined members if our server can read the room history
// TODO: Long term we probably want a history_visibility table which stores eventNID | visibility_enum so we can just
//       pull all events and then filter by that table.
func joinEventsFromHistoryVisibility(
	ctx context.Context, db storage.Database, roomID string, stateEntries []types.StateEntry,
	thisServer gomatrixserverlib.ServerName) ([]types.Event, error) {

	var eventNIDs []types.EventNID
	for _, entry := range stateEntries {
		// Filter the events to retrieve to only keep the membership events
		if entry.EventTypeNID == types.MRoomHistoryVisibilityNID && entry.EventStateKeyNID == types.EmptyStateKeyNID {
			eventNIDs = append(eventNIDs, entry.EventNID)
			break
		}
	}

	// Get all of the events in this state
	stateEvents, err := db.Events(ctx, eventNIDs)
	if err != nil {
		return nil, err
	}
	events := make([]*gomatrixserverlib.Event, len(stateEvents))
	for i := range stateEvents {
		events[i] = stateEvents[i].Event
	}

	// Can we see events in the room?
	canSeeEvents := auth.IsServerAllowed(thisServer, true, events)
	if !canSeeEvents {
		logrus.Infof("ServersAtEvent history not visible to us: %s", auth.HistoryVisibilityForRoom(events))
		return nil, nil
	}
	// get joined members
	info, err := db.RoomInfo(ctx, roomID)
	if err != nil {
		return nil, err
	}
	joinEventNIDs, err := db.GetMembershipEventNIDsForRoom(ctx, info.RoomNID, true, false)
	if err != nil {
		return nil, err
	}
	return db.Events(ctx, joinEventNIDs)
}

func persistEvents(ctx context.Context, db storage.Database, events []*gomatrixserverlib.HeaderedEvent) (types.RoomNID, map[string]types.Event) {
	var roomNID types.RoomNID
	var eventNID types.EventNID
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
		var redactedEventID string
		var redactionEvent *gomatrixserverlib.Event
		eventNID, roomNID, _, redactionEvent, redactedEventID, err = db.StoreEvent(ctx, ev.Unwrap(), authNids, false)
		if err != nil {
			logrus.WithError(err).WithField("event_id", ev.EventID()).Error("Failed to persist event")
			continue
		}
		// If storing this event results in it being redacted, then do so.
		// It's also possible for this event to be a redaction which results in another event being
		// redacted, which we don't care about since we aren't returning it in this backfill.
		if redactedEventID == ev.EventID() {
			eventToRedact := ev.Unwrap()
			redactedEvent, err := eventutil.RedactEvent(redactionEvent, eventToRedact)
			if err != nil {
				logrus.WithError(err).WithField("event_id", ev.EventID()).Error("Failed to redact event")
				continue
			}
			ev = redactedEvent.Headered(ev.RoomVersion)
			events[j] = ev
		}
		backfilledEventMap[ev.EventID()] = types.Event{
			EventNID: eventNID,
			Event:    ev.Unwrap(),
		}
	}
	return roomNID, backfilledEventMap
}
