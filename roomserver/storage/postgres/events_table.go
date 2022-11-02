// Copyright 2017-2018 New Vector Ltd
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

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
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
	auth_event_nids BIGINT[] NOT NULL,
	is_rejected BOOLEAN NOT NULL DEFAULT FALSE
);
`

const insertEventSQL = "" +
	"INSERT INTO roomserver_events AS e (room_nid, event_type_nid, event_state_key_nid, event_id, reference_sha256, auth_event_nids, depth, is_rejected)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8)" +
	" ON CONFLICT ON CONSTRAINT roomserver_event_id_unique DO UPDATE" +
	" SET is_rejected = $8 WHERE e.event_id = $4 AND e.is_rejected = TRUE" +
	" RETURNING event_nid, state_snapshot_nid"

const selectEventSQL = "" +
	"SELECT event_nid, state_snapshot_nid FROM roomserver_events WHERE event_id = $1"

const bulkSelectSnapshotsForEventIDsSQL = "" +
	"SELECT event_id, state_snapshot_nid FROM roomserver_events WHERE event_id = ANY($1)"

// Bulk lookup of events by string ID.
// Sort by the numeric IDs for event type and state key.
// This means we can use binary search to lookup entries by type and state key.
const bulkSelectStateEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	" WHERE event_id = ANY($1)" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

// Bulk lookup of events by string ID that aren't excluded.
// Sort by the numeric IDs for event type and state key.
// This means we can use binary search to lookup entries by type and state key.
const bulkSelectStateEventByIDExcludingRejectedSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	" WHERE event_id = ANY($1) AND is_rejected = FALSE" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

// Bulk look up of events by event NID, optionally filtering by the event type
// or event state key NIDs if provided. (The CARDINALITY check will return true
// if the provided arrays are empty, ergo no filtering).
const bulkSelectStateEventByNIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	" WHERE event_nid = ANY($1)" +
	" AND (CARDINALITY($2::bigint[]) = 0 OR event_type_nid = ANY($2))" +
	" AND (CARDINALITY($3::bigint[]) = 0 OR event_state_key_nid = ANY($3))" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

const bulkSelectStateAtEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, is_rejected FROM roomserver_events" +
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

const bulkSelectUnsentEventNIDSQL = "" +
	"SELECT event_id, event_nid FROM roomserver_events WHERE event_id = ANY($1) AND sent_to_output = FALSE"

const selectMaxEventDepthSQL = "" +
	"SELECT COALESCE(MAX(depth) + 1, 0) FROM roomserver_events WHERE event_nid = ANY($1)"

const selectRoomNIDsForEventNIDsSQL = "" +
	"SELECT event_nid, room_nid FROM roomserver_events WHERE event_nid = ANY($1)"

const selectEventRejectedSQL = "" +
	"SELECT is_rejected FROM roomserver_events WHERE room_nid = $1 AND event_id = $2"

type eventStatements struct {
	insertEventStmt                               *sql.Stmt
	selectEventStmt                               *sql.Stmt
	bulkSelectSnapshotsForEventIDsStmt            *sql.Stmt
	bulkSelectStateEventByIDStmt                  *sql.Stmt
	bulkSelectStateEventByIDExcludingRejectedStmt *sql.Stmt
	bulkSelectStateEventByNIDStmt                 *sql.Stmt
	bulkSelectStateAtEventByIDStmt                *sql.Stmt
	updateEventStateStmt                          *sql.Stmt
	selectEventSentToOutputStmt                   *sql.Stmt
	updateEventSentToOutputStmt                   *sql.Stmt
	selectEventIDStmt                             *sql.Stmt
	bulkSelectStateAtEventAndReferenceStmt        *sql.Stmt
	bulkSelectEventReferenceStmt                  *sql.Stmt
	bulkSelectEventIDStmt                         *sql.Stmt
	bulkSelectEventNIDStmt                        *sql.Stmt
	bulkSelectUnsentEventNIDStmt                  *sql.Stmt
	selectMaxEventDepthStmt                       *sql.Stmt
	selectRoomNIDsForEventNIDsStmt                *sql.Stmt
	selectEventRejectedStmt                       *sql.Stmt
}

func CreateEventsTable(db *sql.DB) error {
	_, err := db.Exec(eventsSchema)
	return err
}

func PrepareEventsTable(db *sql.DB) (tables.Events, error) {
	s := &eventStatements{}

	return s, sqlutil.StatementList{
		{&s.insertEventStmt, insertEventSQL},
		{&s.selectEventStmt, selectEventSQL},
		{&s.bulkSelectSnapshotsForEventIDsStmt, bulkSelectSnapshotsForEventIDsSQL},
		{&s.bulkSelectStateEventByIDStmt, bulkSelectStateEventByIDSQL},
		{&s.bulkSelectStateEventByIDExcludingRejectedStmt, bulkSelectStateEventByIDExcludingRejectedSQL},
		{&s.bulkSelectStateEventByNIDStmt, bulkSelectStateEventByNIDSQL},
		{&s.bulkSelectStateAtEventByIDStmt, bulkSelectStateAtEventByIDSQL},
		{&s.updateEventStateStmt, updateEventStateSQL},
		{&s.updateEventSentToOutputStmt, updateEventSentToOutputSQL},
		{&s.selectEventSentToOutputStmt, selectEventSentToOutputSQL},
		{&s.selectEventIDStmt, selectEventIDSQL},
		{&s.bulkSelectStateAtEventAndReferenceStmt, bulkSelectStateAtEventAndReferenceSQL},
		{&s.bulkSelectEventReferenceStmt, bulkSelectEventReferenceSQL},
		{&s.bulkSelectEventIDStmt, bulkSelectEventIDSQL},
		{&s.bulkSelectEventNIDStmt, bulkSelectEventNIDSQL},
		{&s.bulkSelectUnsentEventNIDStmt, bulkSelectUnsentEventNIDSQL},
		{&s.selectMaxEventDepthStmt, selectMaxEventDepthSQL},
		{&s.selectRoomNIDsForEventNIDsStmt, selectRoomNIDsForEventNIDsSQL},
		{&s.selectEventRejectedStmt, selectEventRejectedSQL},
	}.Prepare(db)
}

func (s *eventStatements) InsertEvent(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventTypeNID types.EventTypeNID,
	eventStateKeyNID types.EventStateKeyNID,
	eventID string,
	referenceSHA256 []byte,
	authEventNIDs []types.EventNID,
	depth int64,
	isRejected bool,
) (types.EventNID, types.StateSnapshotNID, error) {
	var eventNID int64
	var stateNID int64
	stmt := sqlutil.TxStmt(txn, s.insertEventStmt)
	err := stmt.QueryRowContext(
		ctx, int64(roomNID), int64(eventTypeNID), int64(eventStateKeyNID),
		eventID, referenceSHA256, eventNIDsAsArray(authEventNIDs), depth,
		isRejected,
	).Scan(&eventNID, &stateNID)
	return types.EventNID(eventNID), types.StateSnapshotNID(stateNID), err
}

func (s *eventStatements) SelectEvent(
	ctx context.Context, txn *sql.Tx, eventID string,
) (types.EventNID, types.StateSnapshotNID, error) {
	var eventNID int64
	var stateNID int64
	err := s.selectEventStmt.QueryRowContext(ctx, eventID).Scan(&eventNID, &stateNID)
	return types.EventNID(eventNID), types.StateSnapshotNID(stateNID), err
}

func (s *eventStatements) BulkSelectSnapshotsFromEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) (map[types.StateSnapshotNID][]string, error) {
	stmt := sqlutil.TxStmt(txn, s.bulkSelectSnapshotsForEventIDsStmt)

	rows, err := stmt.QueryContext(ctx, pq.Array(eventIDs))
	if err != nil {
		return nil, err
	}

	var eventID string
	var stateNID types.StateSnapshotNID
	result := make(map[types.StateSnapshotNID][]string)
	for rows.Next() {
		if err := rows.Scan(&eventID, &stateNID); err != nil {
			return nil, err
		}
		result[stateNID] = append(result[stateNID], eventID)
	}

	return result, rows.Err()
}

// bulkSelectStateEventByID lookups a list of state events by event ID.
// If not excluding rejected events, and any of the requested events are missing from
// the database it returns a types.MissingEventError. If excluding rejected events,
// the events will be silently omitted without error.
func (s *eventStatements) BulkSelectStateEventByID(
	ctx context.Context, txn *sql.Tx, eventIDs []string, excludeRejected bool,
) ([]types.StateEntry, error) {
	var stmt *sql.Stmt
	if excludeRejected {
		stmt = sqlutil.TxStmt(txn, s.bulkSelectStateEventByIDExcludingRejectedStmt)
	} else {
		stmt = sqlutil.TxStmt(txn, s.bulkSelectStateEventByIDStmt)
	}
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateEventByID: rows.close() failed")
	// We know that we will only get as many results as event IDs
	// because of the unique constraint on event IDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than IDs so we adjust the length of the slice before returning it.
	results := make([]types.StateEntry, 0, len(eventIDs))
	i := 0
	for ; rows.Next(); i++ {
		var result types.StateEntry
		if err = rows.Scan(
			&result.EventTypeNID,
			&result.EventStateKeyNID,
			&result.EventNID,
		); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if !excludeRejected && i != len(eventIDs) {
		// If there are fewer rows returned than IDs then we were asked to lookup event IDs we don't have.
		// We don't know which ones were missing because we don't return the string IDs in the query.
		// However it should be possible debug this by replaying queries or entries from the input kafka logs.
		// If this turns out to be impossible and we do need the debug information here, it would be better
		// to do it as a separate query rather than slowing down/complicating the internal case.
		return nil, types.MissingEventError(
			fmt.Sprintf("storage: state event IDs missing from the database (%d != %d)", i, len(eventIDs)),
		)
	}
	return results, nil
}

// bulkSelectStateEventByNID lookups a list of state events by event NID.
// If any of the requested events are missing from the database it returns a types.MissingEventError
func (s *eventStatements) BulkSelectStateEventByNID(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntry, error) {
	tuples := types.StateKeyTupleSorter(stateKeyTuples)
	sort.Sort(tuples)
	eventTypeNIDArray, eventStateKeyNIDArray := tuples.TypesAndStateKeysAsArrays()
	stmt := sqlutil.TxStmt(txn, s.bulkSelectStateEventByNIDStmt)
	rows, err := stmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs), pq.Int64Array(eventTypeNIDArray), pq.Int64Array(eventStateKeyNIDArray))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateEventByID: rows.close() failed")
	// We know that we will only get as many results as event IDs
	// because of the unique constraint on event IDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than IDs so we adjust the length of the slice before returning it.
	results := make([]types.StateEntry, len(eventNIDs))
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
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return results[:i], nil
}

// bulkSelectStateAtEventByID lookups the state at a list of events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError.
// If we do not have the state for any of the requested events it returns a types.MissingEventError.
func (s *eventStatements) BulkSelectStateAtEventByID(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StateAtEvent, error) {
	stmt := sqlutil.TxStmt(txn, s.bulkSelectStateAtEventByIDStmt)
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateAtEventByID: rows.close() failed")
	results := make([]types.StateAtEvent, len(eventIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(
			&result.EventTypeNID,
			&result.EventStateKeyNID,
			&result.EventNID,
			&result.BeforeStateSnapshotNID,
			&result.IsRejected,
		); err != nil {
			return nil, err
		}
		// Genuine create events are the only case where it's OK to have no previous state.
		isCreate := result.EventTypeNID == types.MRoomCreateNID && result.EventStateKeyNID == 1
		if result.BeforeStateSnapshotNID == 0 && !isCreate {
			return nil, types.MissingStateError(
				fmt.Sprintf("storage: missing state for event NID %d", result.EventNID),
			)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(eventIDs) {
		return nil, types.MissingEventError(
			fmt.Sprintf("storage: event IDs missing from the database (%d != %d)", i, len(eventIDs)),
		)
	}
	return results, nil
}

func (s *eventStatements) UpdateEventState(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {
	stmt := sqlutil.TxStmt(txn, s.updateEventStateStmt)
	_, err := stmt.ExecContext(ctx, int64(eventNID), int64(stateNID))
	return err
}

func (s *eventStatements) SelectEventSentToOutput(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (sentToOutput bool, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectEventSentToOutputStmt)
	err = stmt.QueryRowContext(ctx, int64(eventNID)).Scan(&sentToOutput)
	return
}

func (s *eventStatements) UpdateEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) error {
	stmt := sqlutil.TxStmt(txn, s.updateEventSentToOutputStmt)
	_, err := stmt.ExecContext(ctx, int64(eventNID))
	return err
}

func (s *eventStatements) SelectEventID(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (eventID string, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectEventIDStmt)
	err = stmt.QueryRowContext(ctx, int64(eventNID)).Scan(&eventID)
	return
}

func (s *eventStatements) BulkSelectStateAtEventAndReference(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) ([]types.StateAtEventAndReference, error) {
	stmt := sqlutil.TxStmt(txn, s.bulkSelectStateAtEventAndReferenceStmt)
	rows, err := stmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateAtEventAndReference: rows.close() failed")
	results := make([]types.StateAtEventAndReference, len(eventNIDs))
	i := 0
	var (
		eventTypeNID     int64
		eventStateKeyNID int64
		eventNID         int64
		stateSnapshotNID int64
		eventID          string
		eventSHA256      []byte
	)
	for ; rows.Next(); i++ {
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
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

func (s *eventStatements) BulkSelectEventReference(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) ([]gomatrixserverlib.EventReference, error) {
	rows, err := s.bulkSelectEventReferenceStmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectEventReference: rows.close() failed")
	results := make([]gomatrixserverlib.EventReference, len(eventNIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(&result.EventID, &result.EventSHA256); err != nil {
			return nil, err
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

// bulkSelectEventID returns a map from numeric event ID to string event ID.
func (s *eventStatements) BulkSelectEventID(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (map[types.EventNID]string, error) {
	stmt := sqlutil.TxStmt(txn, s.bulkSelectEventIDStmt)
	rows, err := stmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectEventID: rows.close() failed")
	results := make(map[types.EventNID]string, len(eventNIDs))
	i := 0
	var eventNID int64
	var eventID string
	for ; rows.Next(); i++ {
		if err = rows.Scan(&eventNID, &eventID); err != nil {
			return nil, err
		}
		results[types.EventNID(eventNID)] = eventID
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

// BulkSelectEventNIDs returns a map from string event ID to numeric event ID.
// If an event ID is not in the database then it is omitted from the map.
func (s *eventStatements) BulkSelectEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string) (map[string]types.EventNID, error) {
	return s.bulkSelectEventNID(ctx, txn, eventIDs, false)
}

// BulkSelectEventNIDs returns a map from string event ID to numeric event ID
// only for events that haven't already been sent to the roomserver output.
// If an event ID is not in the database then it is omitted from the map.
func (s *eventStatements) BulkSelectUnsentEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string) (map[string]types.EventNID, error) {
	return s.bulkSelectEventNID(ctx, txn, eventIDs, true)
}

// bulkSelectEventNIDs returns a map from string event ID to numeric event ID.
// If an event ID is not in the database then it is omitted from the map.
func (s *eventStatements) bulkSelectEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string, onlyUnsent bool) (map[string]types.EventNID, error) {
	var stmt *sql.Stmt
	if onlyUnsent {
		stmt = sqlutil.TxStmt(txn, s.bulkSelectUnsentEventNIDStmt)
	} else {
		stmt = sqlutil.TxStmt(txn, s.bulkSelectEventNIDStmt)
	}
	rows, err := stmt.QueryContext(ctx, pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectEventNID: rows.close() failed")
	results := make(map[string]types.EventNID, len(eventIDs))
	var eventID string
	var eventNID int64
	for rows.Next() {
		if err = rows.Scan(&eventID, &eventNID); err != nil {
			return nil, err
		}
		results[eventID] = types.EventNID(eventNID)
	}
	return results, rows.Err()
}

func (s *eventStatements) SelectMaxEventDepth(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (int64, error) {
	var result int64
	stmt := s.selectMaxEventDepthStmt
	err := stmt.QueryRowContext(ctx, eventNIDsAsArray(eventNIDs)).Scan(&result)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (s *eventStatements) SelectRoomNIDsForEventNIDs(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) (map[types.EventNID]types.RoomNID, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomNIDsForEventNIDsStmt)
	rows, err := stmt.QueryContext(ctx, eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectRoomNIDsForEventNIDsStmt: rows.close() failed")
	result := make(map[types.EventNID]types.RoomNID)
	var eventNID types.EventNID
	var roomNID types.RoomNID
	for rows.Next() {
		if err = rows.Scan(&eventNID, &roomNID); err != nil {
			return nil, err
		}
		result[eventNID] = roomNID
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

func (s *eventStatements) SelectEventRejected(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, eventID string,
) (rejected bool, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectEventRejectedStmt)
	err = stmt.QueryRowContext(ctx, roomNID, eventID).Scan(&rejected)
	return
}
