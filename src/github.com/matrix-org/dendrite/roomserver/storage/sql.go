package storage

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type statements struct {
	partitionOffsetStatements
	eventTypeStatements
	eventStateKeyStatements
	roomStatements
	eventStatements
	insertEventJSONStmt            *sql.Stmt
	bulkSelectEventJSONStmt        *sql.Stmt
	insertStateStmt                *sql.Stmt
	bulkSelectStateBlockNIDsStmt   *sql.Stmt
	insertStateDataStmt            *sql.Stmt
	selectNextStateBlockNIDStmt    *sql.Stmt
	bulkSelectStateDataEntriesStmt *sql.Stmt
}

func (s *statements) prepare(db *sql.DB) error {
	var err error

	if err = s.partitionOffsetStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventTypeStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventStateKeyStatements.prepare(db); err != nil {
		return err
	}

	if err = s.roomStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventStatements.prepare(db); err != nil {
		return err
	}

	if err = s.prepareEventJSON(db); err != nil {
		return err
	}

	return nil
}

func (s *statements) prepareEventJSON(db *sql.DB) (err error) {
	_, err = db.Exec(eventJSONSchema)
	if err != nil {
		return
	}
	if s.insertEventJSONStmt, err = db.Prepare(insertEventJSONSQL); err != nil {
		return
	}
	if s.bulkSelectEventJSONStmt, err = db.Prepare(bulkSelectEventJSONSQL); err != nil {
		return
	}
	return
}

const eventJSONSchema = `
-- Stores the JSON for each event. This kept separate from the main events
-- table to keep the rows in the main events table small.
CREATE TABLE IF NOT EXISTS event_json (
    -- Local numeric ID for the event.
    event_nid BIGINT NOT NULL PRIMARY KEY,
    -- The JSON for the event.
    -- Stored as TEXT because this should be valid UTF-8.
    -- Not stored as a JSONB because we always just pull the entire event
    -- so there is no point in postgres parsing it.
    -- Not stored as JSON because we already validate the JSON in the server
    -- so there is no point in postgres validating it.
    -- TODO: Should we be compressing the events with Snappy or DEFLATE?
    event_json TEXT NOT NULL
);
`

const insertEventJSONSQL = "" +
	"INSERT INTO event_json (event_nid, event_json) VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

// Bulk event JSON lookup by numeric event ID.
// Sort by the numeric event ID.
// This means that we can use binary search to lookup by numeric event ID.
const bulkSelectEventJSONSQL = "" +
	"SELECT event_nid, event_json FROM event_json" +
	" WHERE event_nid = ANY($1)" +
	" ORDER BY event_nid ASC"

func (s *statements) insertEventJSON(eventNID types.EventNID, eventJSON []byte) error {
	_, err := s.insertEventJSONStmt.Exec(int64(eventNID), eventJSON)
	return err
}

type eventJSONPair struct {
	EventNID  types.EventNID
	EventJSON []byte
}

func (s *statements) bulkSelectEventJSON(eventNIDs []types.EventNID) ([]eventJSONPair, error) {
	nids := make([]int64, len(eventNIDs))
	for i := range eventNIDs {
		nids[i] = int64(eventNIDs[i])
	}
	rows, err := s.bulkSelectEventJSONStmt.Query(pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// We know that we will only get as many results as event NIDs
	// because of the unique constraint on event NIDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than NIDs so we adjust the length of the slice before returning it.
	results := make([]eventJSONPair, len(eventNIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		var eventNID int64
		if err := rows.Scan(&eventNID, &result.EventJSON); err != nil {
			return nil, err
		}
		result.EventNID = types.EventNID(eventNID)
	}
	return results[:i], nil
}

const stateSchema = `
-- The state of a room before an event.
-- Stored as a list of state_block entries stored in a separate table.
-- The actual state is constructed by combining all the state_block entries
-- referenced by state_block_nids together. If the same state key tuple appears
-- multiple times then the entry from the later state_block clobbers the earlier
-- entries.
-- This encoding format allows us to implement a delta encoding which is useful
-- because room state tends to accumulate small changes over time. Although if
-- the list of deltas becomes too long it becomes more efficient to encode
-- the full state under single state_block_nid.
CREATE SEQUENCE IF NOT EXISTS state_snapshot_nid_seq;
CREATE TABLE IF NOT EXISTS state_snapshots (
    -- Local numeric ID for the state.
    state_snapshot_nid bigint PRIMARY KEY DEFAULT nextval('state_snapshot_nid_seq'),
    -- Local numeric ID of the room this state is for.
    -- Unused in normal operation, but useful for background work or ad-hoc debugging.
    room_nid bigint NOT NULL,
    -- List of state_block_nids, stored sorted by state_block_nid.
    state_block_nids bigint[] NOT NULL
);
`

const insertStateSQL = "" +
	"INSERT INTO state_snapshots (room_nid, state_block_nids)" +
	" VALUES ($1, $2)" +
	" RETURNING state_snapshot_nid"

// Bulk state data NID lookup.
// Sorting by state_snapshot_nid means we can use binary search over the result
// to lookup the state data NIDs for a state snapshot NID.
const bulkSelectStateBlockNIDsSQL = "" +
	"SELECT state_snapshot_nid, state_block_nids FROM state_snapshots" +
	" WHERE state_snapshot_nid = ANY($1) ORDER BY state_snapshot_nid ASC"

func (s *statements) prepareState(db *sql.DB) (err error) {
	_, err = db.Exec(stateSchema)
	if err != nil {
		return
	}
	if s.insertStateStmt, err = db.Prepare(insertStateSQL); err != nil {
		return
	}
	if s.bulkSelectStateBlockNIDsStmt, err = db.Prepare(bulkSelectStateBlockNIDsSQL); err != nil {
		return
	}
	return
}

func (s *statements) insertState(roomNID types.RoomNID, stateBlockNIDs []types.StateBlockNID) (stateNID types.StateSnapshotNID, err error) {
	nids := make([]int64, len(stateBlockNIDs))
	for i := range stateBlockNIDs {
		nids[i] = int64(stateBlockNIDs[i])
	}
	err = s.insertStateStmt.QueryRow(int64(roomNID), pq.Int64Array(nids)).Scan(&stateNID)
	return
}

func (s *statements) bulkSelectStateBlockNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error) {
	nids := make([]int64, len(stateNIDs))
	for i := range stateNIDs {
		nids[i] = int64(stateNIDs[i])
	}
	rows, err := s.bulkSelectStateBlockNIDsStmt.Query(pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]types.StateBlockNIDList, len(stateNIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		var stateBlockNIDs pq.Int64Array
		if err := rows.Scan(&result.StateSnapshotNID, &stateBlockNIDs); err != nil {
			return nil, err
		}
		result.StateBlockNIDs = make([]types.StateBlockNID, len(stateBlockNIDs))
		for k := range stateBlockNIDs {
			result.StateBlockNIDs[k] = types.StateBlockNID(stateBlockNIDs[k])
		}
	}
	if i != len(stateNIDs) {
		return nil, fmt.Errorf("storage: state NIDs missing from the database (%d != %d)", i, len(stateNIDs))
	}
	return results, nil
}

const stateDataSchema = `
-- The state data map.
-- Designed to give enough information to run the state resolution algorithm
-- without hitting the database in the common case.
-- TODO: Is it worth replacing the unique btree index with a covering index so
-- that postgres could lookup the state using an index-only scan?
-- The type and state_key are included in the index to make it easier to
-- lookup a specific (type, state_key) pair for an event. It also makes it easy
-- to read the state for a given state_block_nid ordered by (type, state_key)
-- which in turn makes it easier to merge state data blocks.
CREATE SEQUENCE IF NOT EXISTS state_block_nid_seq;
CREATE TABLE IF NOT EXISTS state_block (
    -- Local numeric ID for this state data.
    state_block_nid bigint NOT NULL,
    event_type_nid bigint NOT NULL,
    event_state_key_nid bigint NOT NULL,
    event_nid bigint NOT NULL,
    UNIQUE (state_block_nid, event_type_nid, event_state_key_nid)
);
`

const insertStateDataSQL = "" +
	"INSERT INTO state_block (state_block_nid, event_type_nid, event_state_key_nid, event_nid)" +
	" VALUES ($1, $2, $3, $4)"

const selectNextStateBlockNIDSQL = "" +
	"SELECT nextval('state_block_nid_seq')"

// Bulk state lookup by numeric event ID.
// Sort by the state_block_nid, event_type_nid, event_state_key_nid
// This means that all the entries for a given state_block_nid will appear
// together in the list and those entries will sorted by event_type_nid
// and event_state_key_nid. This property makes it easier to merge two
// state data blocks together.
const bulkSelectStateDataEntriesSQL = "" +
	"SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
	" FROM state_block WHERE state_block_nid = ANY($1)" +
	" ORDER BY state_block_nid, event_type_nid, event_state_key_nid"

func (s *statements) prepareStateData(db *sql.DB) (err error) {
	_, err = db.Exec(stateDataSchema)
	if err != nil {
		return
	}
	if s.insertStateDataStmt, err = db.Prepare(insertStateDataSQL); err != nil {
		return
	}
	if s.selectNextStateBlockNIDStmt, err = db.Prepare(selectNextStateBlockNIDSQL); err != nil {
		return
	}

	if s.bulkSelectStateDataEntriesStmt, err = db.Prepare(bulkSelectStateDataEntriesSQL); err != nil {
		return
	}
	return
}

func (s *statements) bulkInsertStateData(stateBlockNID types.StateBlockNID, entries []types.StateEntry) error {
	for _, entry := range entries {
		_, err := s.insertStateDataStmt.Exec(
			int64(stateBlockNID),
			int64(entry.EventTypeNID),
			int64(entry.EventStateKeyNID),
			int64(entry.EventNID),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *statements) selectNextStateBlockNID() (types.StateBlockNID, error) {
	var stateBlockNID int64
	err := s.selectNextStateBlockNIDStmt.QueryRow().Scan(&stateBlockNID)
	return types.StateBlockNID(stateBlockNID), err
}

func (s *statements) bulkSelectStateDataEntries(stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error) {
	nids := make([]int64, len(stateBlockNIDs))
	for i := range stateBlockNIDs {
		nids[i] = int64(stateBlockNIDs[i])
	}
	rows, err := s.bulkSelectStateDataEntriesStmt.Query(pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]types.StateEntryList, len(stateBlockNIDs))
	// current is a pointer to the StateEntryList to append the state entries to.
	var current *types.StateEntryList
	i := 0
	for rows.Next() {
		var (
			stateBlockNID    int64
			eventTypeNID     int64
			eventStateKeyNID int64
			eventNID         int64
			entry            types.StateEntry
		)
		if err := rows.Scan(
			&stateBlockNID, &eventTypeNID, &eventStateKeyNID, &eventNID,
		); err != nil {
			return nil, err
		}
		entry.EventTypeNID = types.EventTypeNID(eventTypeNID)
		entry.EventStateKeyNID = types.EventStateKeyNID(eventStateKeyNID)
		entry.EventNID = types.EventNID(eventNID)
		if current == nil || types.StateBlockNID(stateBlockNID) != current.StateBlockNID {
			// The state entry row is for a different state data block to the current one.
			// So we start appending to the next entry in the list.
			current = &results[i]
			current.StateBlockNID = types.StateBlockNID(stateBlockNID)
			i++
		}
		current.StateEntries = append(current.StateEntries, entry)
	}
	if i != len(stateBlockNIDs) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", i, len(stateBlockNIDs))
	}
	return results, nil
}
