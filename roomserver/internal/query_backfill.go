package internal

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/auth"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// backfillRequester implements gomatrixserverlib.BackfillRequester
type backfillRequester struct {
	db         storage.Database
	fedClient  *gomatrixserverlib.FederationClient
	thisServer gomatrixserverlib.ServerName

	// per-request state
	servers                 []gomatrixserverlib.ServerName
	eventIDToBeforeStateIDs map[string][]string
	eventIDMap              map[string]gomatrixserverlib.Event
}

func newBackfillRequester(db storage.Database, fedClient *gomatrixserverlib.FederationClient, thisServer gomatrixserverlib.ServerName) *backfillRequester {
	return &backfillRequester{
		db:                      db,
		fedClient:               fedClient,
		thisServer:              thisServer,
		eventIDToBeforeStateIDs: make(map[string][]string),
		eventIDMap:              make(map[string]gomatrixserverlib.Event),
	}
}

func (b *backfillRequester) StateIDsBeforeEvent(ctx context.Context, targetEvent gomatrixserverlib.HeaderedEvent) ([]string, error) {
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
			FedClient:          b.fedClient,
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

func (b *backfillRequester) calculateNewStateIDs(targetEvent, prevEvent gomatrixserverlib.Event, prevEventStateIDs []string) []string {
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
		if ev.Type() == prevEvent.Type() && ev.StateKey() != nil && *ev.StateKey() == *prevEvent.StateKey() {
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
	event gomatrixserverlib.HeaderedEvent, eventIDs []string) (map[string]*gomatrixserverlib.Event, error) {

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
				result[events[i].EventID()] = &events[i]
				b.eventIDMap[events[i].EventID()] = events[i]
			}
			return result, nil
		}
	}

	c := gomatrixserverlib.FederatedStateProvider{
		FedClient:          b.fedClient,
		RememberAuthEvents: false,
		Server:             b.servers[0],
	}
	result, err := c.StateBeforeEvent(ctx, roomVer, event, eventIDs)
	if err != nil {
		return nil, err
	}
	for eventID, ev := range result {
		b.eventIDMap[eventID] = *ev
	}
	return result, nil
}

// ServersAtEvent is called when trying to determine which server to request from.
// It returns a list of servers which can be queried for backfill requests. These servers
// will be servers that are in the room already. The entries at the beginning are preferred servers
// and will be tried first. An empty list will fail the request.
func (b *backfillRequester) ServersAtEvent(ctx context.Context, roomID, eventID string) (servers []gomatrixserverlib.ServerName) {
	// getMembershipsBeforeEventNID requires a NID, so retrieving the NID for
	// the event is necessary.
	NIDs, err := b.db.EventNIDs(ctx, []string{eventID})
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to get event NID for event")
		return
	}

	stateEntries, err := stateBeforeEvent(ctx, b.db, NIDs[eventID])
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to load state before event")
		return
	}

	// possibly return all joined servers depending on history visiblity
	memberEventsFromVis, err := joinEventsFromHistoryVisibility(ctx, b.db, roomID, stateEntries)
	if err != nil {
		logrus.WithError(err).Error("ServersAtEvent: failed calculate servers from history visibility rules")
		return
	}
	logrus.Infof("ServersAtEvent including %d current events from history visibility", len(memberEventsFromVis))

	// Retrieve all "m.room.member" state events of "join" membership, which
	// contains the list of users in the room before the event, therefore all
	// the servers in it at that moment.
	memberEvents, err := getMembershipsAtState(ctx, b.db, stateEntries, true)
	if err != nil {
		logrus.WithField("event_id", eventID).WithError(err).Error("ServersAtEvent: failed to get memberships before event")
		return
	}
	memberEvents = append(memberEvents, memberEventsFromVis...)

	// Store the server names in a temporary map to avoid duplicates.
	serverSet := make(map[gomatrixserverlib.ServerName]bool)
	for _, event := range memberEvents {
		serverSet[event.Origin()] = true
	}
	for server := range serverSet {
		if server == b.thisServer {
			continue
		}
		servers = append(servers, server)
	}
	b.servers = servers
	return
}

// Backfill performs a backfill request to the given server.
// https://matrix.org/docs/spec/server_server/latest#get-matrix-federation-v1-backfill-roomid
func (b *backfillRequester) Backfill(ctx context.Context, server gomatrixserverlib.ServerName, roomID string,
	fromEventIDs []string, limit int) (*gomatrixserverlib.Transaction, error) {

	tx, err := b.fedClient.Backfill(ctx, server, roomID, limit, fromEventIDs)
	return &tx, err
}

func (b *backfillRequester) ProvideEvents(roomVer gomatrixserverlib.RoomVersion, eventIDs []string) ([]gomatrixserverlib.Event, error) {
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
	events := make([]gomatrixserverlib.Event, len(eventsWithNids))
	for i := range eventsWithNids {
		events[i] = eventsWithNids[i].Event
	}
	return events, nil
}

// joinEventsFromHistoryVisibility returns all CURRENTLY joined members if the provided state indicated a 'shared' history visibility.
// TODO: Long term we probably want a history_visibility table which stores eventNID | visibility_enum so we can just
//       pull all events and then filter by that table.
func joinEventsFromHistoryVisibility(
	ctx context.Context, db storage.Database, roomID string, stateEntries []types.StateEntry) ([]types.Event, error) {

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
	events := make([]gomatrixserverlib.Event, len(stateEvents))
	for i := range stateEvents {
		events[i] = stateEvents[i].Event
	}
	visibility := auth.HistoryVisibilityForRoom(events)
	if visibility != "shared" {
		logrus.Infof("ServersAtEvent history visibility not shared: %s", visibility)
		return nil, nil
	}
	// get joined members
	roomNID, err := db.RoomNID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	joinEventNIDs, err := db.GetMembershipEventNIDsForRoom(ctx, roomNID, true)
	if err != nil {
		return nil, err
	}
	return db.Events(ctx, joinEventNIDs)
}
