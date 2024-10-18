// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2017, 2018 New Vector Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/storage/sqlite3/deltas"
	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/element-hq/dendrite/roomserver/types"
)

const eventsSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_events (
    event_nid INTEGER PRIMARY KEY AUTOINCREMENT,
    room_nid INTEGER NOT NULL,
    event_type_nid INTEGER NOT NULL,
    event_state_key_nid INTEGER NOT NULL,
    sent_to_output BOOLEAN NOT NULL DEFAULT FALSE,
    state_snapshot_nid INTEGER NOT NULL DEFAULT 0,
    depth INTEGER NOT NULL,
    event_id TEXT NOT NULL UNIQUE,
	auth_event_nids TEXT NOT NULL DEFAULT '[]',
	is_rejected BOOLEAN NOT NULL DEFAULT FALSE
  );

-- Create an index which helps in resolving membership events (event_type_nid = 5) - (used for history visibility)
CREATE INDEX IF NOT EXISTS roomserver_events_memberships_idx ON roomserver_events (room_nid, event_state_key_nid) WHERE (event_type_nid = 5);

-- The following indexes are used by bulkSelectStateEventByNIDSQL 
CREATE INDEX IF NOT EXISTS roomserver_event_event_type_nid_idx ON roomserver_events (event_type_nid);
CREATE INDEX IF NOT EXISTS roomserver_event_state_key_nid_idx ON roomserver_events (event_state_key_nid);

`

const insertEventSQL = `
	INSERT INTO roomserver_events (room_nid, event_type_nid, event_state_key_nid, event_id, auth_event_nids, depth, is_rejected)
	  VALUES ($1, $2, $3, $4, $5, $6, $7)
	  ON CONFLICT DO UPDATE
	  SET is_rejected = $7 WHERE is_rejected = 1
	  RETURNING event_nid, state_snapshot_nid;
`

const selectEventSQL = "" +
	"SELECT event_nid, state_snapshot_nid FROM roomserver_events WHERE event_id = $1"

const bulkSelectSnapshotsForEventIDsSQL = "" +
	"SELECT event_id, state_snapshot_nid FROM roomserver_events WHERE event_id IN ($1)"

// Bulk lookup of events by string ID.
// Sort by the numeric IDs for event type and state key.
// This means we can use binary search to lookup entries by type and state key.
const bulkSelectStateEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	" WHERE event_id IN ($1)" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

// Bulk lookup of events by string ID that aren't rejected.
// Sort by the numeric IDs for event type and state key.
// This means we can use binary search to lookup entries by type and state key.
const bulkSelectStateEventByIDExcludingRejectedSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	" WHERE event_id IN ($1) AND is_rejected = 0" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

const bulkSelectStateEventByNIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	" WHERE event_nid IN ($1)"

// Rest of query is built by BulkSelectStateEventByNID

const bulkSelectStateAtEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, is_rejected FROM roomserver_events" +
	" WHERE event_id IN ($1)"

const updateEventStateSQL = "" +
	"UPDATE roomserver_events SET state_snapshot_nid = $1 WHERE event_nid = $2"

const selectEventSentToOutputSQL = "" +
	"SELECT sent_to_output FROM roomserver_events WHERE event_nid = $1"

const updateEventSentToOutputSQL = "" +
	"UPDATE roomserver_events SET sent_to_output = TRUE WHERE event_nid = $1"

const selectEventIDSQL = "" +
	"SELECT event_id FROM roomserver_events WHERE event_nid = $1"

const bulkSelectStateAtEventAndReferenceSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, event_id" +
	" FROM roomserver_events WHERE event_nid IN ($1)"

const bulkSelectEventIDSQL = "" +
	"SELECT event_nid, event_id FROM roomserver_events WHERE event_nid IN ($1)"

const bulkSelectEventNIDSQL = "" +
	"SELECT event_id, event_nid, room_nid FROM roomserver_events WHERE event_id IN ($1)"

const bulkSelectUnsentEventNIDSQL = "" +
	"SELECT event_id, event_nid, room_nid FROM roomserver_events WHERE sent_to_output = 0 AND event_id IN ($1)"

const selectMaxEventDepthSQL = "" +
	"SELECT COALESCE(MAX(depth) + 1, 0) FROM roomserver_events WHERE event_nid IN ($1)"

const selectRoomNIDsForEventNIDsSQL = "" +
	"SELECT event_nid, room_nid FROM roomserver_events WHERE event_nid IN ($1)"

const selectEventRejectedSQL = "" +
	"SELECT is_rejected FROM roomserver_events WHERE room_nid = $1 AND event_id = $2"

const selectRoomsWithEventTypeNIDSQL = `SELECT DISTINCT room_nid FROM roomserver_events WHERE event_type_nid = $1`

type eventStatements struct {
	db                                            *sql.DB
	insertEventStmt                               *sql.Stmt
	selectEventStmt                               *sql.Stmt
	bulkSelectSnapshotsForEventIDsStmt            *sql.Stmt
	bulkSelectStateEventByIDStmt                  *sql.Stmt
	bulkSelectStateEventByIDExcludingRejectedStmt *sql.Stmt
	bulkSelectStateAtEventByIDStmt                *sql.Stmt
	updateEventStateStmt                          *sql.Stmt
	selectEventSentToOutputStmt                   *sql.Stmt
	updateEventSentToOutputStmt                   *sql.Stmt
	selectEventIDStmt                             *sql.Stmt
	bulkSelectStateAtEventAndReferenceStmt        *sql.Stmt
	bulkSelectEventIDStmt                         *sql.Stmt
	selectEventRejectedStmt                       *sql.Stmt
	selectRoomsWithEventTypeNIDStmt               *sql.Stmt
	//bulkSelectEventNIDStmt               *sql.Stmt
	//bulkSelectUnsentEventNIDStmt         *sql.Stmt
	//selectRoomNIDsForEventNIDsStmt       *sql.Stmt
}

func CreateEventsTable(db *sql.DB) error {
	_, err := db.Exec(eventsSchema)
	if err != nil {
		return err
	}

	// check if the column exists
	var cName string
	migrationName := "roomserver: drop column reference_sha from roomserver_events"
	err = db.QueryRowContext(context.Background(), `SELECT p.name FROM sqlite_master AS m JOIN pragma_table_info(m.name) AS p WHERE m.name = 'roomserver_events' AND p.name = 'reference_sha256'`).Scan(&cName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) { // migration was already executed, as the column was removed
			if err = sqlutil.InsertMigration(context.Background(), db, migrationName); err != nil {
				return fmt.Errorf("unable to manually insert migration '%s': %w", migrationName, err)
			}
			return nil
		}
		return err
	}

	m := sqlutil.NewMigrator(db)
	m.AddMigrations([]sqlutil.Migration{
		{
			Version: migrationName,
			Up:      deltas.UpDropEventReferenceSHA,
		},
	}...)
	return m.Up(context.Background())
}

func PrepareEventsTable(db *sql.DB) (tables.Events, error) {
	s := &eventStatements{
		db: db,
	}

	return s, sqlutil.StatementList{
		{&s.insertEventStmt, insertEventSQL},
		{&s.selectEventStmt, selectEventSQL},
		{&s.bulkSelectSnapshotsForEventIDsStmt, bulkSelectSnapshotsForEventIDsSQL},
		{&s.bulkSelectStateEventByIDStmt, bulkSelectStateEventByIDSQL},
		{&s.bulkSelectStateEventByIDExcludingRejectedStmt, bulkSelectStateEventByIDExcludingRejectedSQL},
		{&s.bulkSelectStateAtEventByIDStmt, bulkSelectStateAtEventByIDSQL},
		{&s.updateEventStateStmt, updateEventStateSQL},
		{&s.updateEventSentToOutputStmt, updateEventSentToOutputSQL},
		{&s.selectEventSentToOutputStmt, selectEventSentToOutputSQL},
		{&s.selectEventIDStmt, selectEventIDSQL},
		{&s.bulkSelectStateAtEventAndReferenceStmt, bulkSelectStateAtEventAndReferenceSQL},
		{&s.bulkSelectEventIDStmt, bulkSelectEventIDSQL},
		//{&s.bulkSelectEventNIDStmt, bulkSelectEventNIDSQL},
		//{&s.bulkSelectUnsentEventNIDStmt, bulkSelectUnsentEventNIDSQL},
		//{&s.selectRoomNIDForEventNIDStmt, selectRoomNIDForEventNIDSQL},
		{&s.selectEventRejectedStmt, selectEventRejectedSQL},
		{&s.selectRoomsWithEventTypeNIDStmt, selectRoomsWithEventTypeNIDSQL},
	}.Prepare(db)
}

func (s *eventStatements) InsertEvent(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventTypeNID types.EventTypeNID,
	eventStateKeyNID types.EventStateKeyNID,
	eventID string,
	authEventNIDs []types.EventNID,
	depth int64,
	isRejected bool,
) (types.EventNID, types.StateSnapshotNID, error) {
	// attempt to insert: the last_row_id is the event NID
	var eventNID int64
	var stateNID int64
	insertStmt := sqlutil.TxStmt(txn, s.insertEventStmt)
	err := insertStmt.QueryRowContext(
		ctx, int64(roomNID), int64(eventTypeNID), int64(eventStateKeyNID),
		eventID, eventNIDsAsArray(authEventNIDs), depth, isRejected,
	).Scan(&eventNID, &stateNID)
	return types.EventNID(eventNID), types.StateSnapshotNID(stateNID), err
}

func (s *eventStatements) SelectEvent(
	ctx context.Context, txn *sql.Tx, eventID string,
) (types.EventNID, types.StateSnapshotNID, error) {
	var eventNID int64
	var stateNID int64
	selectStmt := sqlutil.TxStmt(txn, s.selectEventStmt)
	err := selectStmt.QueryRowContext(ctx, eventID).Scan(&eventNID, &stateNID)
	return types.EventNID(eventNID), types.StateSnapshotNID(stateNID), err
}

func (s *eventStatements) BulkSelectSnapshotsFromEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) (map[types.StateSnapshotNID][]string, error) {
	qry := strings.Replace(bulkSelectSnapshotsForEventIDsSQL, "($1)", sqlutil.QueryVariadic(len(eventIDs)), 1)
	stmt, err := s.db.Prepare(qry)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, stmt, "BulkSelectSnapshotsFromEventIDs: stmt.close() failed")

	params := make([]interface{}, len(eventIDs))
	for i := range eventIDs {
		params[i] = eventIDs[i]
	}

	rows, err := stmt.QueryContext(ctx, params...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "BulkSelectSnapshotsFromEventIDs: rows.close() failed")

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
	///////////////
	var sql string
	if excludeRejected {
		sql = bulkSelectStateEventByIDExcludingRejectedSQL
	} else {
		sql = bulkSelectStateEventByIDSQL
	}
	iEventIDs := make([]interface{}, len(eventIDs))
	for k, v := range eventIDs {
		iEventIDs[k] = v
	}
	selectOrig := strings.Replace(sql, "($1)", sqlutil.QueryVariadic(len(iEventIDs)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	defer selectPrep.Close() // nolint:errcheck
	selectStmt := sqlutil.TxStmt(txn, selectPrep)
	///////////////

	rows, err := selectStmt.QueryContext(ctx, iEventIDs...)
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
	return results, err
}

// bulkSelectStateEventByID lookups a list of state events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError
func (s *eventStatements) BulkSelectStateEventByNID(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntry, error) {
	tuples := types.StateKeyTupleSorter(stateKeyTuples)
	sort.Sort(tuples)
	eventTypeNIDArray, eventStateKeyNIDArray := tuples.TypesAndStateKeysAsArrays()
	params := make([]interface{}, 0, len(eventNIDs)+len(eventTypeNIDArray)+len(eventStateKeyNIDArray))
	selectOrig := strings.Replace(bulkSelectStateEventByNIDSQL, "($1)", sqlutil.QueryVariadic(len(eventNIDs)), 1)
	for _, v := range eventNIDs {
		params = append(params, v)
	}
	if len(eventTypeNIDArray) > 0 {
		selectOrig += " AND event_type_nid IN " + sqlutil.QueryVariadicOffset(len(eventTypeNIDArray), len(params))
		for _, v := range eventTypeNIDArray {
			params = append(params, v)
		}
	}
	if len(eventStateKeyNIDArray) > 0 {
		selectOrig += " AND event_state_key_nid IN " + sqlutil.QueryVariadicOffset(len(eventStateKeyNIDArray), len(params))
		for _, v := range eventStateKeyNIDArray {
			params = append(params, v)
		}
	}
	selectOrig += " ORDER BY event_type_nid, event_state_key_nid ASC"
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, fmt.Errorf("s.db.Prepare: %w", err)
	}
	defer selectPrep.Close() // nolint:errcheck
	selectStmt := sqlutil.TxStmt(txn, selectPrep)
	rows, err := selectStmt.QueryContext(ctx, params...)
	if err != nil {
		return nil, fmt.Errorf("selectStmt.QueryContext: %w", err)
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
	return results[:i], rows.Err()
}

// bulkSelectStateAtEventByID lookups the state at a list of events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError.
// If we do not have the state for any of the requested events it returns a types.MissingEventError.
func (s *eventStatements) BulkSelectStateAtEventByID(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StateAtEvent, error) {
	///////////////
	iEventIDs := make([]interface{}, len(eventIDs))
	for k, v := range eventIDs {
		iEventIDs[k] = v
	}
	selectOrig := strings.Replace(bulkSelectStateAtEventByIDSQL, "($1)", sqlutil.QueryVariadic(len(iEventIDs)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	defer selectPrep.Close() // nolint:errcheck
	selectStmt := sqlutil.TxStmt(txn, selectPrep)
	///////////////
	rows, err := selectStmt.QueryContext(ctx, iEventIDs...)
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
	return results, err
}

func (s *eventStatements) UpdateEventState(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {
	stmt := sqlutil.TxStmt(txn, s.updateEventStateStmt)
	_, err := stmt.ExecContext(ctx, int64(stateNID), int64(eventNID))
	return err
}

func (s *eventStatements) SelectEventSentToOutput(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (sentToOutput bool, err error) {
	selectStmt := sqlutil.TxStmt(txn, s.selectEventSentToOutputStmt)
	err = selectStmt.QueryRowContext(ctx, int64(eventNID)).Scan(&sentToOutput)
	return
}

func (s *eventStatements) UpdateEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) error {
	updateStmt := sqlutil.TxStmt(txn, s.updateEventSentToOutputStmt)
	_, err := updateStmt.ExecContext(ctx, int64(eventNID))
	return err
}

func (s *eventStatements) SelectEventID(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (eventID string, err error) {
	selectStmt := sqlutil.TxStmt(txn, s.selectEventIDStmt)
	err = selectStmt.QueryRowContext(ctx, int64(eventNID)).Scan(&eventID)
	return
}

func (s *eventStatements) BulkSelectStateAtEventAndReference(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) ([]types.StateAtEventAndReference, error) {
	///////////////
	iEventNIDs := make([]interface{}, len(eventNIDs))
	for k, v := range eventNIDs {
		iEventNIDs[k] = v
	}
	selectOrig := strings.Replace(bulkSelectStateAtEventAndReferenceSQL, "($1)", sqlutil.QueryVariadic(len(iEventNIDs)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	defer selectPrep.Close() // nolint:errcheck
	selectStmt := sqlutil.TxStmt(txn, selectPrep)
	//////////////

	rows, err := sqlutil.TxStmt(txn, selectStmt).QueryContext(ctx, iEventNIDs...)
	if err != nil {
		return nil, fmt.Errorf("sqlutil.TxStmt.QueryContext: %w", err)
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
	)
	for ; rows.Next(); i++ {
		if err = rows.Scan(
			&eventTypeNID, &eventStateKeyNID, &eventNID, &stateSnapshotNID, &eventID,
		); err != nil {
			return nil, err
		}
		result := &results[i]
		result.EventTypeNID = types.EventTypeNID(eventTypeNID)
		result.EventStateKeyNID = types.EventStateKeyNID(eventStateKeyNID)
		result.EventNID = types.EventNID(eventNID)
		result.BeforeStateSnapshotNID = types.StateSnapshotNID(stateSnapshotNID)
		result.EventID = eventID
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
	///////////////
	iEventNIDs := make([]interface{}, len(eventNIDs))
	for k, v := range eventNIDs {
		iEventNIDs[k] = v
	}
	selectOrig := strings.Replace(bulkSelectEventIDSQL, "($1)", sqlutil.QueryVariadic(len(iEventNIDs)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	defer selectPrep.Close() // nolint:errcheck
	selectStmt := sqlutil.TxStmt(txn, selectPrep)
	///////////////

	rows, err := selectStmt.QueryContext(ctx, iEventNIDs...)
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
func (s *eventStatements) BulkSelectEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string) (map[string]types.EventMetadata, error) {
	return s.bulkSelectEventNID(ctx, txn, eventIDs, false)
}

// BulkSelectEventNIDs returns a map from string event ID to numeric event ID
// only for events that haven't already been sent to the roomserver output.
// If an event ID is not in the database then it is omitted from the map.
func (s *eventStatements) BulkSelectUnsentEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string) (map[string]types.EventMetadata, error) {
	return s.bulkSelectEventNID(ctx, txn, eventIDs, true)
}

// bulkSelectEventNIDs returns a map from string event ID to numeric event ID.
// If an event ID is not in the database then it is omitted from the map.
func (s *eventStatements) bulkSelectEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string, onlyUnsent bool) (map[string]types.EventMetadata, error) {
	///////////////
	iEventIDs := make([]interface{}, len(eventIDs))
	for k, v := range eventIDs {
		iEventIDs[k] = v
	}
	var selectOrig string
	if onlyUnsent {
		selectOrig = strings.Replace(bulkSelectUnsentEventNIDSQL, "($1)", sqlutil.QueryVariadic(len(iEventIDs)), 1)
	} else {
		selectOrig = strings.Replace(bulkSelectEventNIDSQL, "($1)", sqlutil.QueryVariadic(len(iEventIDs)), 1)
	}
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	defer selectPrep.Close() // nolint:errcheck
	selectStmt := sqlutil.TxStmt(txn, selectPrep)
	///////////////
	rows, err := selectStmt.QueryContext(ctx, iEventIDs...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectEventNID: rows.close() failed")
	results := make(map[string]types.EventMetadata, len(eventIDs))
	var eventID string
	var eventNID int64
	var roomNID int64
	for rows.Next() {
		if err = rows.Scan(&eventID, &eventNID, &roomNID); err != nil {
			return nil, err
		}
		results[eventID] = types.EventMetadata{
			EventNID: types.EventNID(eventNID),
			RoomNID:  types.RoomNID(roomNID),
		}
	}
	return results, rows.Err()
}

func (s *eventStatements) SelectMaxEventDepth(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (int64, error) {
	var result int64
	iEventIDs := make([]interface{}, len(eventNIDs))
	for i, v := range eventNIDs {
		iEventIDs[i] = v
	}
	sqlStr := strings.Replace(selectMaxEventDepthSQL, "($1)", sqlutil.QueryVariadic(len(iEventIDs)), 1)
	sqlPrep, err := s.db.Prepare(sqlStr)
	if err != nil {
		return 0, err
	}
	defer internal.CloseAndLogIfError(ctx, sqlPrep, "sqlPrep.close() failed")
	err = sqlutil.TxStmt(txn, sqlPrep).QueryRowContext(ctx, iEventIDs...).Scan(&result)
	if err != nil {
		return 0, fmt.Errorf("sqlutil.TxStmt.QueryRowContext: %w", err)
	}
	return result, nil
}

func (s *eventStatements) SelectRoomNIDsForEventNIDs(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) (map[types.EventNID]types.RoomNID, error) {
	sqlStr := strings.Replace(selectRoomNIDsForEventNIDsSQL, "($1)", sqlutil.QueryVariadic(len(eventNIDs)), 1)
	sqlPrep, err := s.db.Prepare(sqlStr)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, sqlPrep, "sqlPrep.close() failed")
	sqlStmt := sqlutil.TxStmt(txn, sqlPrep)
	iEventNIDs := make([]interface{}, len(eventNIDs))
	for i, v := range eventNIDs {
		iEventNIDs[i] = v
	}
	rows, err := sqlStmt.QueryContext(ctx, iEventNIDs...)
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
	return result, rows.Err()
}

func eventNIDsAsArray(eventNIDs []types.EventNID) string {
	if eventNIDs == nil {
		eventNIDs = []types.EventNID{} // don't store 'null' in the DB
	}
	b, _ := json.Marshal(eventNIDs)
	return string(b)
}

func (s *eventStatements) SelectEventRejected(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, eventID string,
) (rejected bool, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectEventRejectedStmt)
	err = stmt.QueryRowContext(ctx, roomNID, eventID).Scan(&rejected)
	return
}

func (s *eventStatements) SelectRoomsWithEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventTypeNID types.EventTypeNID,
) ([]types.RoomNID, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomsWithEventTypeNIDStmt)
	rows, err := stmt.QueryContext(ctx, eventTypeNID)
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRoomsWithEventTypeNID: rows.close() failed")
	if err != nil {
		return nil, err
	}

	var roomNIDs []types.RoomNID
	var roomNID types.RoomNID
	for rows.Next() {
		if err := rows.Scan(&roomNID); err != nil {
			return nil, err
		}
		roomNIDs = append(roomNIDs, roomNID)
	}

	return roomNIDs, rows.Err()
}
