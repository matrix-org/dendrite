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
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const eventsSchema = `
-- The events table holds metadata for each event, the actual JSON is stored
-- separately to keep the size of the rows small.
CREATE SEQUENCE IF NOT EXISTS roomserver_event_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_events (
    -- Local numeric ID for the event.
    event_nid BIGINT PRIMARY KEY DEFAULT nextval('roomserver_event_nid_seq'),
    -- Local numeric ID for the room the event is in.
    -- This is never 0.
    room_nid BIGINT NOT NULL,
    -- Local numeric ID for the type of the event.
    -- This is never 0.
    event_type_nid BIGINT NOT NULL,
    -- Local numeric ID for the state_key of the event
    -- This is 0 if the event is not a state event.
    event_state_key_nid BIGINT NOT NULL,
    -- Whether the event has been written to the output log.
    sent_to_output BOOLEAN NOT NULL DEFAULT FALSE,
    -- Local numeric ID for the state at the event.
    -- This is 0 if we don't know the state at the event.
    -- If the state is not 0 then this event is part of the contiguous
    -- part of the event graph
    -- Since many different events can have the same state we store the
    -- state into a separate state table and refer to it by numeric ID.
    state_snapshot_nid BIGINT NOT NULL DEFAULT 0,
    -- Depth of the event in the event graph.
    depth BIGINT NOT NULL,
    -- The textual event id.
    -- Used to lookup the numeric ID when processing requests.
    -- Needed for state resolution.
    -- An event may only appear in this table once.
    event_id TEXT NOT NULL CONSTRAINT roomserver_event_id_unique UNIQUE,
    -- The sha256 reference hash for the event.
    -- Needed for setting reference hashes when sending new events.
    reference_sha256 BYTEA NOT NULL,
    -- A list of numeric IDs for events that can authenticate this event.
    auth_event_nids BIGINT[] NOT NULL
);
`

const insertEventSQL = "" +
	"INSERT INTO roomserver_events (room_nid, event_type_nid, event_state_key_nid, event_id, reference_sha256, auth_event_nids, depth)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7)" +
	" ON CONFLICT ON CONSTRAINT roomserver_event_id_unique" +
	" DO NOTHING" +
	" RETURNING event_nid, state_snapshot_nid"

const selectEventSQL = "" +
	"SELECT event_nid, state_snapshot_nid FROM roomserver_events WHERE event_id = $1"

// Bulk lookup of events by string ID.
// Sort by the numeric IDs for event type and state key.
// This means we can use binary search to lookup entries by type and state key.
const bulkSelectStateEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	" WHERE event_id = ANY($1)" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

const bulkSelectStateAtEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid FROM roomserver_events" +
	" WHERE event_id = ANY($1)"

const updateEventStateSQL = "" +
	"UPDATE roomserver_events SET state_snapshot_nid = $2 WHERE event_nid = $1"

const selectEventSentToOutputSQL = "" +
	"SELECT sent_to_output FROM roomserver_events WHERE event_nid = $1"

const updateEventSentToOutputSQL = "" +
	"UPDATE roomserver_events SET sent_to_output = TRUE WHERE event_nid = $1"

const selectEventIDSQL = "" +
	"SELECT event_id FROM roomserver_events WHERE event_nid = $1"

const bulkSelectStateAtEventAndReferenceSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, event_id, reference_sha256" +
	" FROM roomserver_events WHERE event_nid = ANY($1)"

const bulkSelectEventReferenceSQL = "" +
	"SELECT event_id, reference_sha256 FROM roomserver_events WHERE event_nid = ANY($1)"

const bulkSelectEventIDSQL = "" +
	"SELECT event_nid, event_id FROM roomserver_events WHERE event_nid = ANY($1)"

const bulkSelectEventNIDSQL = "" +
	"SELECT event_id, event_nid FROM roomserver_events WHERE event_id = ANY($1)"

const selectMaxEventDepthSQL = "" +
	"SELECT COALESCE(MAX(depth) + 1, 0) FROM roomserver_events WHERE event_nid = ANY($1)"

type eventStatements struct {
	insertEventStmt                        *sql.Stmt
	selectEventStmt                        *sql.Stmt
	bulkSelectStateEventByIDStmt           *sql.Stmt
	bulkSelectStateAtEventByIDStmt         *sql.Stmt
	updateEventStateStmt                   *sql.Stmt
	selectEventSentToOutputStmt            *sql.Stmt
	updateEventSentToOutputStmt            *sql.Stmt
	selectEventIDStmt                      *sql.Stmt
	bulkSelectStateAtEventAndReferenceStmt *sql.Stmt
	bulkSelectEventReferenceStmt           *sql.Stmt
	bulkSelectEventIDStmt                  *sql.Stmt
	bulkSelectEventNIDStmt                 *sql.Stmt
	selectMaxEventDepthStmt                *sql.Stmt
}

func (s *eventStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(eventsSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertEventStmt, insertEventSQL},
		{&s.selectEventStmt, selectEventSQL},
		{&s.bulkSelectStateEventByIDStmt, bulkSelectStateEventByIDSQL},
		{&s.bulkSelectStateAtEventByIDStmt, bulkSelectStateAtEventByIDSQL},
		{&s.updateEventStateStmt, updateEventStateSQL},
		{&s.updateEventSentToOutputStmt, updateEventSentToOutputSQL},
		{&s.selectEventSentToOutputStmt, selectEventSentToOutputSQL},
		{&s.selectEventIDStmt, selectEventIDSQL},
		{&s.bulkSelectStateAtEventAndReferenceStmt, bulkSelectStateAtEventAndReferenceSQL},
		{&s.bulkSelectEventReferenceStmt, bulkSelectEventReferenceSQL},
		{&s.bulkSelectEventIDStmt, bulkSelectEventIDSQL},
		{&s.bulkSelectEventNIDStmt, bulkSelectEventNIDSQL},
		{&s.selectMaxEventDepthStmt, selectMaxEventDepthSQL},
	}.prepare(db)
}

func (s *eventStatements) insertEvent(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID,
	eventTypeNID types.EventTypeNID,
	eventStateKeyNID types.EventStateKeyNID,
	eventID string,
	referenceSHA256 []byte,
	authEventNIDs []types.EventNID,
	depth int64,
) (types.EventNID, types.StateSnapshotNID, error) {
	var eventNID int64
	var stateNID int64
	stmt := common.TxStmt(txn, s.insertEventStmt)
	err := stmt.QueryRowContext(
		ctx, int64(roomNID), int64(eventTypeNID), int64(eventStateKeyNID),
		eventID, referenceSHA256, eventNIDsAsArray(authEventNIDs), depth,
	).Scan(&eventNID, &stateNID)
	return types.EventNID(eventNID), types.StateSnapshotNID(stateNID), err
}

func (s *eventStatements) selectEvent(
	ctx context.Context, txn *sql.Tx, eventID string,
) (types.EventNID, types.StateSnapshotNID, error) {
	var eventNID int64
	var stateNID int64
	stmt := common.TxStmt(txn, s.selectEventStmt)
	err := stmt.QueryRowContext(ctx, eventID).Scan(&eventNID, &stateNID)
	return types.EventNID(eventNID), types.StateSnapshotNID(stateNID), err
}

// bulkSelectStateEventByID lookups a list of state events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError
func (s *eventStatements) bulkSelectStateEventByID(
	ctx context.Context, eventIDs []string,
) ([]types.StateEntry, error) {
	rows, err := s.bulkSelectStateEventByIDStmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	// We know that we will only get as many results as event IDs
	// because of the unique constraint on event IDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than IDs so we adjust the length of the slice before returning it.
	results := make([]types.StateEntry, len(eventIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(
			&result.EventTypeNID,
			&result.EventStateKeyNID,
			&result.EventNID,
		); err != nil {
			return nil, err
		}
	}
	if i != len(eventIDs) {
		// If there are fewer rows returned than IDs then we were asked to lookup event IDs we don't have.
		// We don't know which ones were missing because we don't return the string IDs in the query.
		// However it should be possible debug this by replaying queries or entries from the input kafka logs.
		// If this turns out to be impossible and we do need the debug information here, it would be better
		// to do it as a separate query rather than slowing down/complicating the common case.
		return nil, types.MissingEventError(
			fmt.Sprintf("storage: state event IDs missing from the database (%d != %d)", i, len(eventIDs)),
		)
	}
	return results, err
}

// bulkSelectStateAtEventByID lookups the state at a list of events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError.
// If we do not have the state for any of the requested events it returns a types.MissingEventError.
func (s *eventStatements) bulkSelectStateAtEventByID(
	ctx context.Context, eventIDs []string,
) ([]types.StateAtEvent, error) {
	rows, err := s.bulkSelectStateAtEventByIDStmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make([]types.StateAtEvent, len(eventIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(
			&result.EventTypeNID,
			&result.EventStateKeyNID,
			&result.EventNID,
			&result.BeforeStateSnapshotNID,
		); err != nil {
			return nil, err
		}
		if result.BeforeStateSnapshotNID == 0 {
			return nil, types.MissingEventError(
				fmt.Sprintf("storage: missing state for event NID %d", result.EventNID),
			)
		}
	}
	if i != len(eventIDs) {
		return nil, types.MissingEventError(
			fmt.Sprintf("storage: event IDs missing from the database (%d != %d)", i, len(eventIDs)),
		)
	}
	return results, err
}

func (s *eventStatements) updateEventState(
	ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {
	_, err := s.updateEventStateStmt.ExecContext(ctx, int64(eventNID), int64(stateNID))
	return err
}

func (s *eventStatements) selectEventSentToOutput(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (sentToOutput bool, err error) {
	stmt := common.TxStmt(txn, s.selectEventSentToOutputStmt)
	err = stmt.QueryRowContext(ctx, int64(eventNID)).Scan(&sentToOutput)
	return
}

func (s *eventStatements) updateEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) error {
	stmt := common.TxStmt(txn, s.updateEventSentToOutputStmt)
	_, err := stmt.ExecContext(ctx, int64(eventNID))
	return err
}

func (s *eventStatements) selectEventID(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (eventID string, err error) {
	stmt := common.TxStmt(txn, s.selectEventIDStmt)
	err = stmt.QueryRowContext(ctx, int64(eventNID)).Scan(&eventID)
	return
}

func (s *eventStatements) bulkSelectStateAtEventAndReference(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) ([]types.StateAtEventAndReference, error) {
	stmt := common.TxStmt(txn, s.bulkSelectStateAtEventAndReferenceStmt)
	rows, err := stmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make([]types.StateAtEventAndReference, len(eventNIDs))
	i := 0
	for ; rows.Next(); i++ {
		var (
			eventTypeNID     int64
			eventStateKeyNID int64
			eventNID         int64
			stateSnapshotNID int64
			eventID          string
			eventSHA256      []byte
		)
		if err = rows.Scan(
			&eventTypeNID, &eventStateKeyNID, &eventNID, &stateSnapshotNID, &eventID, &eventSHA256,
		); err != nil {
			return nil, err
		}
		result := &results[i]
		result.EventTypeNID = types.EventTypeNID(eventTypeNID)
		result.EventStateKeyNID = types.EventStateKeyNID(eventStateKeyNID)
		result.EventNID = types.EventNID(eventNID)
		result.BeforeStateSnapshotNID = types.StateSnapshotNID(stateSnapshotNID)
		result.EventID = eventID
		result.EventSHA256 = eventSHA256
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

func (s *eventStatements) bulkSelectEventReference(
	ctx context.Context, eventNIDs []types.EventNID,
) ([]gomatrixserverlib.EventReference, error) {
	rows, err := s.bulkSelectEventReferenceStmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make([]gomatrixserverlib.EventReference, len(eventNIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(&result.EventID, &result.EventSHA256); err != nil {
			return nil, err
		}
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

// bulkSelectEventID returns a map from numeric event ID to string event ID.
func (s *eventStatements) bulkSelectEventID(ctx context.Context, eventNIDs []types.EventNID) (map[types.EventNID]string, error) {
	rows, err := s.bulkSelectEventIDStmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make(map[types.EventNID]string, len(eventNIDs))
	i := 0
	for ; rows.Next(); i++ {
		var eventNID int64
		var eventID string
		if err = rows.Scan(&eventNID, &eventID); err != nil {
			return nil, err
		}
		results[types.EventNID(eventNID)] = eventID
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

// bulkSelectEventNIDs returns a map from string event ID to numeric event ID.
// If an event ID is not in the database then it is omitted from the map.
func (s *eventStatements) bulkSelectEventNID(ctx context.Context, eventIDs []string) (map[string]types.EventNID, error) {
	rows, err := s.bulkSelectEventNIDStmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make(map[string]types.EventNID, len(eventIDs))
	for rows.Next() {
		var eventID string
		var eventNID int64
		if err = rows.Scan(&eventID, &eventNID); err != nil {
			return nil, err
		}
		results[eventID] = types.EventNID(eventNID)
	}
	return results, nil
}

func (s *eventStatements) selectMaxEventDepth(ctx context.Context, eventNIDs []types.EventNID) (int64, error) {
	var result int64
	stmt := s.selectMaxEventDepthStmt
	err := stmt.QueryRowContext(ctx, eventNIDsAsArray(eventNIDs)).Scan(&result)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func eventNIDsAsArray(eventNIDs []types.EventNID) pq.Int64Array {
	nids := make([]int64, len(eventNIDs))
	for i := range eventNIDs {
		nids[i] = int64(eventNIDs[i])
	}
	return nids
}
