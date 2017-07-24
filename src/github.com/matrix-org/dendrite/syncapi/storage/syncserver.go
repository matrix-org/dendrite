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

package storage

import (
	"database/sql"
	"fmt"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type stateDelta struct {
	roomID      string
	stateEvents []gomatrixserverlib.Event
	membership  string
	// The stream position of the latest membership event for this user, if applicable.
	// Can be 0 if there is no membership event in this delta.
	membershipPos types.StreamPosition
}

// Same as gomatrixserverlib.Event but also has the stream position for this event.
type streamEvent struct {
	gomatrixserverlib.Event
	streamPosition types.StreamPosition
}

// SyncServerDatabase represents a sync server database
type SyncServerDatabase struct {
	db         *sql.DB
	partitions common.PartitionOffsetStatements
	events     outputRoomEventsStatements
	roomstate  currentRoomStateStatements
}

// NewSyncServerDatabase creates a new sync server database
func NewSyncServerDatabase(dataSourceName string) (*SyncServerDatabase, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	partitions := common.PartitionOffsetStatements{}
	if err = partitions.Prepare(db); err != nil {
		return nil, err
	}
	events := outputRoomEventsStatements{}
	if err = events.prepare(db); err != nil {
		return nil, err
	}
	state := currentRoomStateStatements{}
	if err := state.prepare(db); err != nil {
		return nil, err
	}
	return &SyncServerDatabase{db, partitions, events, state}, nil
}

// AllJoinedUsersInRooms returns a map of room ID to a list of all joined user IDs.
func (d *SyncServerDatabase) AllJoinedUsersInRooms() (map[string][]string, error) {
	return d.roomstate.selectJoinedUsers()
}

// Events lookups a list of event by their event ID.
// Returns a list of events matching the requested IDs found in the database.
// If an event is not found in the database then it will be omitted from the list.
// Returns an error if there was a problem talking with the database
func (d *SyncServerDatabase) Events(eventIDs []string) ([]gomatrixserverlib.Event, error) {
	streamEvents, err := d.events.selectEvents(nil, eventIDs)
	if err != nil {
		return nil, err
	}
	return streamEventsToEvents(streamEvents), nil
}

// WriteEvent into the database. It is not safe to call this function from multiple goroutines, as it would create races
// when generating the stream position for this event. Returns the sync stream position for the inserted event.
// Returns an error if there was a problem inserting this event.
func (d *SyncServerDatabase) WriteEvent(
	ev *gomatrixserverlib.Event, addStateEvents []gomatrixserverlib.Event, addStateEventIDs, removeStateEventIDs []string,
) (streamPos types.StreamPosition, returnErr error) {
	returnErr = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		var err error
		pos, err := d.events.insertEvent(txn, ev, addStateEventIDs, removeStateEventIDs)
		if err != nil {
			return err
		}
		streamPos = types.StreamPosition(pos)

		if len(addStateEvents) == 0 {
			// If the event is a m.room.member event, and has unsigned content,
			// we need to save it as it is very likely to be a membership update
			// (e.g. if a user updates their profile)
			if ev.Type() == "m.room.member" && len(ev.Unsigned()) > 0 {
				addStateEvents = append(addStateEvents, *ev)
			} else if len(removeStateEventIDs) == 0 {
				// Nothing to do, the event may have just been a message event.
				return nil
			}
		}

		return d.updateRoomState(txn, removeStateEventIDs, addStateEvents, streamPos)
	})
	return
}

func (d *SyncServerDatabase) updateRoomState(
	txn *sql.Tx, removedEventIDs []string, addedEvents []gomatrixserverlib.Event, streamPos types.StreamPosition,
) error {
	// remove first, then add, as we do not ever delete state, but do replace state which is a remove followed by an add.
	for _, eventID := range removedEventIDs {
		if err := d.roomstate.deleteRoomStateByEventID(txn, eventID); err != nil {
			return err
		}
	}

	for _, event := range addedEvents {
		if event.StateKey() == nil {
			// ignore non state events
			continue
		}
		var membership *string
		if event.Type() == "m.room.member" {
			value, err := event.Membership()
			if err != nil {
				return err
			}
			membership = &value
		}
		if err := d.roomstate.upsertRoomState(txn, event, membership, int64(streamPos)); err != nil {
			return err
		}
	}

	return nil
}

// GetMembershipEvent returns the Matrix join event for a given user into a given room
func (d *SyncServerDatabase) GetMembershipEvent(roomID string, userID string) (*gomatrixserverlib.Event, error) {
	return d.roomstate.selectJoinEventForUser(roomID, userID)
}

// PartitionOffsets implements common.PartitionStorer
func (d *SyncServerDatabase) PartitionOffsets(topic string) ([]common.PartitionOffset, error) {
	return d.partitions.SelectPartitionOffsets(topic)
}

// SetPartitionOffset implements common.PartitionStorer
func (d *SyncServerDatabase) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.partitions.UpsertPartitionOffset(topic, partition, offset)
}

// SyncStreamPosition returns the latest position in the sync stream. Returns 0 if there are no events yet.
func (d *SyncServerDatabase) SyncStreamPosition() (types.StreamPosition, error) {
	id, err := d.events.selectMaxID(nil)
	if err != nil {
		return types.StreamPosition(0), err
	}
	return types.StreamPosition(id), nil
}

// IncrementalSync returns all the data needed in order to create an incremental sync response.
func (d *SyncServerDatabase) IncrementalSync(userID string, fromPos, toPos types.StreamPosition, numRecentEventsPerRoom int) (res *types.Response, returnErr error) {
	returnErr = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		// Work out which rooms to return in the response. This is done by getting not only the currently
		// joined rooms, but also which rooms have membership transitions for this user between the 2 stream positions.
		// This works out what the 'state' key should be for each room as well as which membership block
		// to put the room into.
		deltas, err := d.getStateDeltas(txn, fromPos, toPos, userID)
		if err != nil {
			return err
		}

		res = types.NewResponse(toPos)
		for _, delta := range deltas {
			endPos := toPos
			if delta.membershipPos > 0 && delta.membership == "leave" {
				// make sure we don't leak recent events after the leave event.
				// TODO: History visibility makes this somewhat complex to handle correctly. For example:
				// TODO: This doesn't work for join -> leave in a single /sync request (see events prior to join).
				// TODO: This will fail on join -> leave -> sensitive msg -> join -> leave
				//       in a single /sync request
				// This is all "okay" assuming history_visibility == "shared" which it is by default.
				endPos = delta.membershipPos
			}
			recentStreamEvents, err := d.events.selectRecentEvents(txn, delta.roomID, fromPos, endPos, numRecentEventsPerRoom)
			if err != nil {
				return err
			}
			recentEvents := streamEventsToEvents(recentStreamEvents)
			delta.stateEvents = removeDuplicates(delta.stateEvents, recentEvents) // roll back

			switch delta.membership {
			case "join":
				jr := types.NewJoinResponse()
				jr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
				jr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
				jr.State.Events = gomatrixserverlib.ToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
				res.Rooms.Join[delta.roomID] = *jr
			case "leave":
				fallthrough // transitions to leave are the same as ban
			case "ban":
				// TODO: recentEvents may contain events that this user is not allowed to see because they are
				//       no longer in the room.
				lr := types.NewLeaveResponse()
				lr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
				lr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
				lr.State.Events = gomatrixserverlib.ToClientEvents(delta.stateEvents, gomatrixserverlib.FormatSync)
				res.Rooms.Leave[delta.roomID] = *lr
			}
		}

		// TODO: This should be done in getStateDeltas
		return d.addInvitesToResponse(txn, userID, res)
	})
	return
}

// CompleteSync a complete /sync API response for the given user.
func (d *SyncServerDatabase) CompleteSync(userID string, numRecentEventsPerRoom int) (res *types.Response, returnErr error) {
	// This needs to be all done in a transaction as we need to do multiple SELECTs, and we need to have
	// a consistent view of the database throughout. This includes extracting the sync stream position.
	// This does have the unfortunate side-effect that all the matrixy logic resides in this function,
	// but it's better to not hide the fact that this is being done in a transaction.
	returnErr = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		// Get the current stream position which we will base the sync response on.
		id, err := d.events.selectMaxID(txn)
		if err != nil {
			return err
		}
		pos := types.StreamPosition(id)

		// Extract room state and recent events for all rooms the user is joined to.
		roomIDs, err := d.roomstate.selectRoomIDsWithMembership(txn, userID, "join")
		if err != nil {
			return err
		}

		// Build up a /sync response. Add joined rooms.
		res = types.NewResponse(pos)
		for _, roomID := range roomIDs {
			stateEvents, err := d.roomstate.selectCurrentState(txn, roomID)
			if err != nil {
				return err
			}
			// TODO: When filters are added, we may need to call this multiple times to get enough events.
			//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
			recentStreamEvents, err := d.events.selectRecentEvents(
				txn, roomID, types.StreamPosition(0), pos, numRecentEventsPerRoom,
			)
			if err != nil {
				return err
			}
			recentEvents := streamEventsToEvents(recentStreamEvents)

			stateEvents = removeDuplicates(stateEvents, recentEvents)
			jr := types.NewJoinResponse()
			jr.Timeline.Events = gomatrixserverlib.ToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
			jr.Timeline.Limited = true
			jr.State.Events = gomatrixserverlib.ToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
			res.Rooms.Join[roomID] = *jr
		}

		return d.addInvitesToResponse(txn, userID, res)
	})
	return
}

func (d *SyncServerDatabase) addInvitesToResponse(txn *sql.Tx, userID string, res *types.Response) error {
	// Add invites - TODO: This will break over federation as they won't be in the current state table according to Mark.
	roomIDs, err := d.roomstate.selectRoomIDsWithMembership(txn, userID, "invite")
	if err != nil {
		return err
	}
	for _, roomID := range roomIDs {
		ir := types.NewInviteResponse()
		// TODO: invite_state. The state won't be in the current state table in cases where you get invited over federation
		res.Rooms.Invite[roomID] = *ir
	}
	return nil
}

// fetchStateEvents converts the set of event IDs into a set of events. It will fetch any which are missing from the database.
// Returns a map of room ID to list of events.
func (d *SyncServerDatabase) fetchStateEvents(txn *sql.Tx, roomIDToEventIDSet map[string]map[string]bool, eventIDToEvent map[string]streamEvent) (map[string][]streamEvent, error) {
	stateBetween := make(map[string][]streamEvent)
	missingEvents := make(map[string][]string)
	for roomID, ids := range roomIDToEventIDSet {
		events := stateBetween[roomID]
		for id, need := range ids {
			if !need {
				continue // deleted state
			}
			e, ok := eventIDToEvent[id]
			if ok {
				events = append(events, e)
			} else {
				m := missingEvents[roomID]
				m = append(m, id)
				missingEvents[roomID] = m
			}
		}
		stateBetween[roomID] = events
	}

	if len(missingEvents) > 0 {
		// This happens when add_state_ids has an event ID which is not in the provided range.
		// We need to explicitly fetch them.
		allMissingEventIDs := []string{}
		for _, missingEvIDs := range missingEvents {
			allMissingEventIDs = append(allMissingEventIDs, missingEvIDs...)
		}
		evs, err := d.fetchMissingStateEvents(txn, allMissingEventIDs)
		if err != nil {
			return nil, err
		}
		// we know we got them all otherwise an error would've been returned, so just loop the events
		for _, ev := range evs {
			roomID := ev.RoomID()
			stateBetween[roomID] = append(stateBetween[roomID], ev)
		}
	}
	return stateBetween, nil
}

func (d *SyncServerDatabase) fetchMissingStateEvents(txn *sql.Tx, eventIDs []string) ([]streamEvent, error) {
	// Fetch from the events table first so we pick up the stream ID for the
	// event.
	events, err := d.events.selectEvents(txn, eventIDs)
	if err != nil {
		return nil, err
	}

	have := map[string]bool{}
	for _, event := range events {
		have[event.EventID()] = true
	}
	var missing []string
	for _, eventID := range eventIDs {
		if !have[eventID] {
			missing = append(missing, eventID)
		}
	}
	if len(missing) == 0 {
		return events, nil
	}

	// If they are missing from the events table then they should be state
	// events that we received from outside the main event stream.
	// These should be in the room state table.
	stateEvents, err := d.roomstate.selectEventsWithEventIDs(txn, missing)

	if err != nil {
		return nil, err
	}
	if len(stateEvents) != len(missing) {
		return nil, fmt.Errorf("failed to map all event IDs to events: (got %d, wanted %d)", len(stateEvents), len(missing))
	}
	for _, e := range stateEvents {
		events = append(events, e)
	}
	return events, nil
}

func (d *SyncServerDatabase) getStateDeltas(txn *sql.Tx, fromPos, toPos types.StreamPosition, userID string) ([]stateDelta, error) {
	// Implement membership change algorithm: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L821
	// - Get membership list changes for this user in this sync response
	// - For each room which has membership list changes:
	//     * Check if the room is 'newly joined' (insufficient to just check for a join event because we allow dupe joins TODO).
	//       If it is, then we need to send the full room state down (and 'limited' is always true).
	//     * Check if user is still CURRENTLY invited to the room. If so, add room to 'invited' block.
	//     * Check if the user is CURRENTLY (TODO) left/banned. If so, add room to 'archived' block.
	// - Get all CURRENTLY joined rooms, and add them to 'joined' block.
	var deltas []stateDelta

	// get all the state events ever between these two positions
	stateNeeded, eventMap, err := d.events.selectStateInRange(txn, fromPos, toPos)
	if err != nil {
		return nil, err
	}
	state, err := d.fetchStateEvents(txn, stateNeeded, eventMap)
	if err != nil {
		return nil, err
	}

	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			// TODO: Currently this will incorrectly add rooms which were ALREADY joined but they sent another no-op join event.
			//       We should be checking if the user was already joined at fromPos and not proceed if so. As a result of this,
			//       dupe join events will result in the entire room state coming down to the client again. This is added in
			//       the 'state' part of the response though, so is transparent modulo bandwidth concerns as it is not added to
			//       the timeline.
			if membership := getMembershipFromEvent(&ev.Event, userID); membership != "" {
				if membership == "join" {
					// send full room state down instead of a delta
					var allState []gomatrixserverlib.Event
					allState, err = d.roomstate.selectCurrentState(txn, roomID)
					if err != nil {
						return nil, err
					}
					s := make([]streamEvent, len(allState))
					for i := 0; i < len(s); i++ {
						s[i] = streamEvent{allState[i], types.StreamPosition(0)}
					}
					state[roomID] = s
					continue // we'll add this room in when we do joined rooms
				}

				deltas = append(deltas, stateDelta{
					membership:    membership,
					membershipPos: ev.streamPosition,
					stateEvents:   streamEventsToEvents(stateStreamEvents),
					roomID:        roomID,
				})
				break
			}
		}
	}

	// Add in currently joined rooms
	joinedRoomIDs, err := d.roomstate.selectRoomIDsWithMembership(txn, userID, "join")
	if err != nil {
		return nil, err
	}
	for _, joinedRoomID := range joinedRoomIDs {
		deltas = append(deltas, stateDelta{
			membership:  "join",
			stateEvents: streamEventsToEvents(state[joinedRoomID]),
			roomID:      joinedRoomID,
		})
	}

	return deltas, nil
}

func streamEventsToEvents(in []streamEvent) []gomatrixserverlib.Event {
	out := make([]gomatrixserverlib.Event, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[i].Event
	}
	return out
}

// There may be some overlap where events in stateEvents are already in recentEvents, so filter
// them out so we don't include them twice in the /sync response. They should be in recentEvents
// only, so clients get to the correct state once they have rolled forward.
func removeDuplicates(stateEvents, recentEvents []gomatrixserverlib.Event) []gomatrixserverlib.Event {
	for _, recentEv := range recentEvents {
		if recentEv.StateKey() == nil {
			continue // not a state event
		}
		// TODO: This is a linear scan over all the current state events in this room. This will
		//       be slow for big rooms. We should instead sort the state events by event ID  (ORDER BY)
		//       then do a binary search to find matching events, similar to what roomserver does.
		for j := 0; j < len(stateEvents); j++ {
			if stateEvents[j].EventID() == recentEv.EventID() {
				// overwrite the element to remove with the last element then pop the last element.
				// This is orders of magnitude faster than re-slicing, but doesn't preserve ordering
				// (we don't care about the order of stateEvents)
				stateEvents[j] = stateEvents[len(stateEvents)-1]
				stateEvents = stateEvents[:len(stateEvents)-1]
				break // there shouldn't be multiple events with the same event ID
			}
		}
	}
	return stateEvents
}

// getMembershipFromEvent returns the value of content.membership iff the event is a state event
// with type 'm.room.member' and state_key of userID. Otherwise, an empty string is returned.
func getMembershipFromEvent(ev *gomatrixserverlib.Event, userID string) string {
	if ev.Type() == "m.room.member" && ev.StateKeyEquals(userID) {
		membership, err := ev.Membership()
		if err != nil {
			return ""
		}
		return membership
	}
	return ""
}
