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

	log "github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const outputRoomEventsSchema = `
-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS output_room_events (
    -- An incrementing ID which denotes the position in the log that this event resides at.
    -- NB: 'serial' makes no guarantees to increment by 1 every time, only that it increments.
    --     This isn't a problem for us since we just want to order by this field.
    id BIGSERIAL PRIMARY KEY,
    -- The event ID for the event
    event_id TEXT NOT NULL,
    -- The 'room_id' key for the event.
    room_id TEXT NOT NULL,
    -- The JSON for the event. Stored as TEXT because this should be valid UTF-8.
    event_json TEXT NOT NULL,
    -- A list of event IDs which represent a delta of added/removed room state. This can be NULL
    -- if there is no delta.
    add_state_ids TEXT[],
    remove_state_ids TEXT[]
);
-- for event selection
CREATE UNIQUE INDEX IF NOT EXISTS event_id_idx ON output_room_events(event_id);
`

const insertEventSQL = "" +
	"INSERT INTO output_room_events (room_id, event_id, event_json, add_state_ids, remove_state_ids) VALUES ($1, $2, $3, $4, $5) RETURNING id"

const selectEventsSQL = "" +
	"SELECT id, event_json FROM output_room_events WHERE event_id = ANY($1)"

const selectRecentEventsSQL = "" +
	"SELECT id, event_json FROM output_room_events WHERE room_id = $1 AND id > $2 AND id <= $3 ORDER BY id DESC LIMIT $4"

const selectMaxIDSQL = "" +
	"SELECT MAX(id) FROM output_room_events"

// In order for us to apply the state updates correctly, rows need to be ordered in the order they were received (id).
const selectStateInRangeSQL = "" +
	"SELECT id, event_json, add_state_ids, remove_state_ids FROM output_room_events" +
	" WHERE (id > $1 AND id <= $2) AND (add_state_ids IS NOT NULL OR remove_state_ids IS NOT NULL)" +
	" ORDER BY id ASC"

type outputRoomEventsStatements struct {
	insertEventStmt        *sql.Stmt
	selectEventsStmt       *sql.Stmt
	selectMaxIDStmt        *sql.Stmt
	selectRecentEventsStmt *sql.Stmt
	selectStateInRangeStmt *sql.Stmt
}

func (s *outputRoomEventsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(outputRoomEventsSchema)
	if err != nil {
		return
	}
	if s.insertEventStmt, err = db.Prepare(insertEventSQL); err != nil {
		return
	}
	if s.selectEventsStmt, err = db.Prepare(selectEventsSQL); err != nil {
		return
	}
	if s.selectMaxIDStmt, err = db.Prepare(selectMaxIDSQL); err != nil {
		return
	}
	if s.selectRecentEventsStmt, err = db.Prepare(selectRecentEventsSQL); err != nil {
		return
	}
	if s.selectStateInRangeStmt, err = db.Prepare(selectStateInRangeSQL); err != nil {
		return
	}
	return
}

// StateBetween returns the state events between the two given stream positions, exclusive of oldPos, inclusive of newPos.
// Results are bucketed based on the room ID. If the same state is overwritten multiple times between the
// two positions, only the most recent state is returned.
func (s *outputRoomEventsStatements) StateBetween(txn *sql.Tx, oldPos, newPos types.StreamPosition) (map[string][]streamEvent, error) {
	rows, err := txn.Stmt(s.selectStateInRangeStmt).Query(oldPos, newPos)
	if err != nil {
		return nil, err
	}
	// Fetch all the state change events for all rooms between the two positions then loop each event and:
	//  - Keep a cache of the event by ID (99% of state change events are for the event itself)
	//  - For each room ID, build up an array of event IDs which represents cumulative adds/removes
	// For each room, map cumulative event IDs to events and return. This may need to a batch SELECT based on event ID
	// if they aren't in the event ID cache. We don't handle state deletion yet.
	eventIDToEvent := make(map[string]streamEvent)

	// RoomID => A set (map[string]bool) of state event IDs which are between the two positions
	stateNeeded := make(map[string]map[string]bool)

	for rows.Next() {
		var (
			streamPos  int64
			eventBytes []byte
			addIDs     pq.StringArray
			delIDs     pq.StringArray
		)
		if err := rows.Scan(&streamPos, &eventBytes, &addIDs, &delIDs); err != nil {
			return nil, err
		}
		// Sanity check for deleted state and whine if we see it. We don't need to do anything
		// since it'll just mark the event as not being needed.
		if len(addIDs) < len(delIDs) {
			log.WithFields(log.Fields{
				"since":   oldPos,
				"current": newPos,
				"adds":    addIDs,
				"dels":    delIDs,
			}).Warn("StateBetween: ignoring deleted state")
		}

		// TODO: Handle redacted events
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false)
		if err != nil {
			return nil, err
		}
		needSet := stateNeeded[ev.RoomID()]
		if needSet == nil { // make set if required
			needSet = make(map[string]bool)
		}
		for _, id := range delIDs {
			needSet[id] = false
		}
		for _, id := range addIDs {
			needSet[id] = true
		}
		stateNeeded[ev.RoomID()] = needSet

		eventIDToEvent[ev.EventID()] = streamEvent{ev, types.StreamPosition(streamPos)}
	}

	return s.fetchStateEvents(txn, stateNeeded, eventIDToEvent)
}

// fetchStateEvents converts the set of event IDs into a set of events. It will fetch any which are missing from the database.
// Returns a map of room ID to list of events.
func (s *outputRoomEventsStatements) fetchStateEvents(txn *sql.Tx, roomIDToEventIDSet map[string]map[string]bool, eventIDToEvent map[string]streamEvent) (map[string][]streamEvent, error) {
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
		evs, err := s.Events(txn, allMissingEventIDs)
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

// MaxID returns the ID of the last inserted event in this table. 'txn' is optional. If it is not supplied,
// then this function should only ever be used at startup, as it will race with inserting events if it is
// done afterwards. If there are no inserted events, 0 is returned.
func (s *outputRoomEventsStatements) MaxID(txn *sql.Tx) (id int64, err error) {
	stmt := s.selectMaxIDStmt
	if txn != nil {
		stmt = txn.Stmt(stmt)
	}
	var nullableID sql.NullInt64
	err = stmt.QueryRow().Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}

// InsertEvent into the output_room_events table. addState and removeState are an optional list of state event IDs. Returns the position
// of the inserted event.
func (s *outputRoomEventsStatements) InsertEvent(txn *sql.Tx, event *gomatrixserverlib.Event, addState, removeState []string) (streamPos int64, err error) {
	err = txn.Stmt(s.insertEventStmt).QueryRow(
		event.RoomID(), event.EventID(), event.JSON(), pq.StringArray(addState), pq.StringArray(removeState),
	).Scan(&streamPos)
	return
}

// RecentEventsInRoom returns the most recent events in the given room, up to a maximum of 'limit'.
func (s *outputRoomEventsStatements) RecentEventsInRoom(txn *sql.Tx, roomID string, fromPos, toPos types.StreamPosition, limit int) ([]streamEvent, error) {
	rows, err := s.selectRecentEventsStmt.Query(roomID, fromPos, toPos, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events, err := rowsToEvents(rows)
	if err != nil {
		return nil, err
	}
	// reverse the order because [0] is the newest event due to the ORDER BY in SQL-land. The reverse order makes [0] the oldest event,
	// which is correct for /sync responses.
	return reverseEvents(events), nil
}

// Events returns the events for the given event IDs. Returns an error if any one of the event IDs given are missing
// from the database.
func (s *outputRoomEventsStatements) Events(txn *sql.Tx, eventIDs []string) ([]streamEvent, error) {
	rows, err := txn.Stmt(s.selectEventsStmt).Query(pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result, err := rowsToEvents(rows)
	if err != nil {
		return nil, err
	}

	if len(result) != len(eventIDs) {
		return nil, fmt.Errorf("failed to map all event IDs to events: (got %d, wanted %d)", len(result), len(eventIDs))
	}
	return result, nil
}

func rowsToEvents(rows *sql.Rows) ([]streamEvent, error) {
	var result []streamEvent
	for rows.Next() {
		var (
			streamPos  int64
			eventBytes []byte
		)
		if err := rows.Scan(&streamPos, &eventBytes); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false)
		if err != nil {
			return nil, err
		}
		result = append(result, streamEvent{ev, types.StreamPosition(streamPos)})
	}
	return result, nil
}

func reverseEvents(input []streamEvent) (output []streamEvent) {
	for i := len(input) - 1; i >= 0; i-- {
		output = append(output, input[i])
	}
	return
}
