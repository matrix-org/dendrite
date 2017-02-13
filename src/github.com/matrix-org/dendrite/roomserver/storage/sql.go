package storage

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type statements struct {
	selectPartitionOffsetsStmt     *sql.Stmt
	upsertPartitionOffsetStmt      *sql.Stmt
	insertEventTypeNIDStmt         *sql.Stmt
	selectEventTypeNIDStmt         *sql.Stmt
	insertEventStateKeyNIDStmt     *sql.Stmt
	selectEventStateKeyNIDStmt     *sql.Stmt
	bulkSelectEventStateKeyNIDStmt *sql.Stmt
	insertRoomNIDStmt              *sql.Stmt
	selectRoomNIDStmt              *sql.Stmt
	insertEventStmt                *sql.Stmt
	bulkSelectStateEventByIDStmt   *sql.Stmt
	bulkSelectStateAtEventByIDStmt *sql.Stmt
	updateEventStateStmt           *sql.Stmt
	insertEventJSONStmt            *sql.Stmt
	bulkSelectEventJSONStmt        *sql.Stmt
	insertStateStmt                *sql.Stmt
	bulkSelectStateDataNIDsStmt    *sql.Stmt
	insertStateDataStmt            *sql.Stmt
	selectNextStateDataNIDStmt     *sql.Stmt
	bulkSelectStateDataEntriesStmt *sql.Stmt
}

func (s *statements) prepare(db *sql.DB) error {
	var err error

	if err = s.preparePartitionOffsets(db); err != nil {
		return err
	}

	if err = s.prepareEventTypes(db); err != nil {
		return err
	}

	if err = s.prepareEventStateKeys(db); err != nil {
		return err
	}

	if err = s.prepareRooms(db); err != nil {
		return err
	}

	if err = s.prepareEvents(db); err != nil {
		return err
	}

	if err = s.prepareEventJSON(db); err != nil {
		return err
	}

	return nil
}

func (s *statements) preparePartitionOffsets(db *sql.DB) (err error) {
	_, err = db.Exec(partitionOffsetsSchema)
	if err != nil {
		return
	}
	if s.selectPartitionOffsetsStmt, err = db.Prepare(selectPartitionOffsetsSQL); err != nil {
		return
	}
	if s.upsertPartitionOffsetStmt, err = db.Prepare(upsertPartitionOffsetsSQL); err != nil {
		return
	}
	return
}

const partitionOffsetsSchema = `
-- The offsets that the server has processed up to.
CREATE TABLE IF NOT EXISTS partition_offsets (
    -- The name of the topic.
    topic TEXT NOT NULL,
    -- The 32-bit partition ID
    partition INTEGER NOT NULL,
    -- The 64-bit offset.
    partition_offset BIGINT NOT NULL,
    CONSTRAINT topic_partition_unique UNIQUE (topic, partition)
);
`

const selectPartitionOffsetsSQL = "" +
	"SELECT partition, partition_offset FROM partition_offsets WHERE topic = $1"

const upsertPartitionOffsetsSQL = "" +
	"INSERT INTO partition_offsets (topic, partition, partition_offset) VALUES ($1, $2, $3)" +
	" ON CONFLICT ON CONSTRAINT topic_partition_unique" +
	" DO UPDATE SET partition_offset = $3"

func (s *statements) selectPartitionOffsets(topic string) ([]types.PartitionOffset, error) {
	rows, err := s.selectPartitionOffsetsStmt.Query(topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []types.PartitionOffset
	for rows.Next() {
		var offset types.PartitionOffset
		if err := rows.Scan(&offset.Partition, &offset.Offset); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (s *statements) upsertPartitionOffset(topic string, partition int32, offset int64) error {
	_, err := s.upsertPartitionOffsetStmt.Exec(topic, partition, offset)
	return err
}

func (s *statements) prepareEventTypes(db *sql.DB) (err error) {
	_, err = db.Exec(eventTypesSchema)
	if err != nil {
		return
	}
	if s.insertEventTypeNIDStmt, err = db.Prepare(insertEventTypeNIDSQL); err != nil {
		return
	}
	if s.selectEventTypeNIDStmt, err = db.Prepare(selectEventTypeNIDSQL); err != nil {
		return
	}
	return
}

const eventTypesSchema = `
-- Numeric versions of the event "type"s. Event types tend to be taken from a
-- small common pool. Assigning each a numeric ID should reduce the amount of
-- data that needs to be stored and fetched from the database.
-- It also means that many operations can work with int64 arrays rather than
-- string arrays which may help reduce GC pressure.
-- Well known event types are pre-assigned numeric IDs:
--   1 -> m.room.create
--   2 -> m.room.power_levels
--   3 -> m.room.join_rules
--   4 -> m.room.third_party_invite
--   5 -> m.room.member
--   6 -> m.room.redaction
--   7 -> m.room.history_visibility
-- Picking well-known numeric IDs for the events types that require special
-- attention during state conflict resolution means that we write that code
-- using numeric constants.
-- It also means that the numeric IDs for common event types should be
-- consistent between different instances which might make ad-hoc debugging
-- easier.
-- Other event types are automatically assigned numeric IDs starting from 2**16.
-- This leaves room to add more pre-assigned numeric IDs and clearly separates
-- the automatically assigned IDs from the pre-assigned IDs.
CREATE SEQUENCE IF NOT EXISTS event_type_nid_seq START 65536;
CREATE TABLE IF NOT EXISTS event_types (
    -- Local numeric ID for the event type.
    event_type_nid BIGINT PRIMARY KEY DEFAULT nextval('event_type_nid_seq'),
    -- The string event_type.
    event_type TEXT NOT NULL CONSTRAINT event_type_unique UNIQUE
);
INSERT INTO event_types (event_type_nid, event_type) VALUES
    (1, 'm.room.create'),
    (2, 'm.room.power_levels'),
    (3, 'm.room.join_rules'),
    (4, 'm.room.third_party_invite'),
    (5, 'm.room.member'),
    (6, 'm.room.redaction'),
    (7, 'm.room.history_visibility') ON CONFLICT DO NOTHING;
`

// Assign a new numeric event type ID.
// The usual case is that the event type is not in the database.
// In that case the ID will be assigned using the next value from the sequence.
// We use `RETURNING` to tell postgres to return the assigned ID.
// But it's possible that the type was added in a query that raced with us.
// This will result in a conflict on the event_type_unique constraint.
// We peform a update that does nothing rather that doing nothing at all because
// postgres won't return anything unless we touch a row in the table.
const insertEventTypeNIDSQL = "" +
	"INSERT INTO event_types (event_type) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT event_type_unique" +
	" DO UPDATE SET event_type = $1" +
	" RETURNING (event_type_nid)"

const selectEventTypeNIDSQL = "" +
	"SELECT event_type_nid FROM event_types WHERE event_type = $1"

func (s *statements) insertEventTypeNID(eventType string) (eventTypeNID int64, err error) {
	err = s.insertEventTypeNIDStmt.QueryRow(eventType).Scan(&eventTypeNID)
	return
}

func (s *statements) selectEventTypeNID(eventType string) (eventTypeNID int64, err error) {
	err = s.selectEventTypeNIDStmt.QueryRow(eventType).Scan(&eventTypeNID)
	return
}

func (s *statements) prepareEventStateKeys(db *sql.DB) (err error) {
	_, err = db.Exec(eventStateKeysSchema)
	if err != nil {
		return
	}
	if s.insertEventStateKeyNIDStmt, err = db.Prepare(insertEventStateKeyNIDSQL); err != nil {
		return
	}
	if s.selectEventStateKeyNIDStmt, err = db.Prepare(selectEventStateKeyNIDSQL); err != nil {
		return
	}
	if s.bulkSelectEventStateKeyNIDStmt, err = db.Prepare(bulkSelectEventStateKeyNIDSQL); err != nil {
		return
	}
	return
}

const eventStateKeysSchema = `
-- Numeric versions of the event "state_key"s. State keys tend to be reused so
-- assigning each string a numeric ID should reduce the amount of data that
-- needs to be stored and fetched from the database.
-- It also means that many operations can work with int64 arrays rather than
-- string arrays which may help reduce GC pressure.
-- Well known state keys are pre-assigned numeric IDs:
--   1 -> "" (the empty string)
-- Other state keys are automatically assigned numeric IDs starting from 2**16.
-- This leaves room to add more pre-assigned numeric IDs and clearly separates
-- the automatically assigned IDs from the pre-assigned IDs.
CREATE SEQUENCE IF NOT EXISTS event_state_key_nid_seq START 65536;
CREATE TABLE IF NOT EXISTS event_state_keys (
    -- Local numeric ID for the state key.
    event_state_key_nid BIGINT PRIMARY KEY DEFAULT nextval('event_state_key_nid_seq'),
    event_state_key TEXT NOT NULL CONSTRAINT event_state_key_unique UNIQUE
);
INSERT INTO event_state_keys (event_state_key_nid, event_state_key) VALUES
    (1, '') ON CONFLICT DO NOTHING;
`

// Same as insertEventTypeNIDSQL
const insertEventStateKeyNIDSQL = "" +
	"INSERT INTO event_state_keys (event_state_key) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT event_state_key_unique" +
	" DO UPDATE SET event_state_key = $1" +
	" RETURNING (event_state_key_nid)"

const selectEventStateKeyNIDSQL = "" +
	"SELECT event_state_key_nid FROM event_state_keys WHERE event_state_key = $1"

// Bulk lookup from string state key to numeric ID for that state key.
// Takes an array of strings as the query parameter.
const bulkSelectEventStateKeyNIDSQL = "" +
	"SELECT event_state_key, event_state_key_nid FROM event_state_keys" +
	" WHERE event_state_key = ANY($1)"

func (s *statements) insertEventStateKeyNID(eventStateKey string) (eventStateKeyNID int64, err error) {
	err = s.insertEventStateKeyNIDStmt.QueryRow(eventStateKey).Scan(&eventStateKeyNID)
	return
}

func (s *statements) selectEventStateKeyNID(eventStateKey string) (eventStateKeyNID int64, err error) {
	err = s.selectEventStateKeyNIDStmt.QueryRow(eventStateKey).Scan(&eventStateKeyNID)
	return
}

func (s *statements) bulkSelectEventStateKeyNID(eventStateKeys []string) (map[string]int64, error) {
	rows, err := s.bulkSelectEventStateKeyNIDStmt.Query(pq.StringArray(eventStateKeys))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64, len(eventStateKeys))
	for rows.Next() {
		var stateKey string
		var stateKeyNID int64
		if err := rows.Scan(&stateKey, &stateKeyNID); err != nil {
			return nil, err
		}
		result[stateKey] = stateKeyNID
	}
	return result, nil
}

func (s *statements) prepareRooms(db *sql.DB) (err error) {
	_, err = db.Exec(roomsSchema)
	if err != nil {
		return
	}
	if s.insertRoomNIDStmt, err = db.Prepare(insertRoomNIDSQL); err != nil {
		return
	}
	if s.selectRoomNIDStmt, err = db.Prepare(selectRoomNIDSQL); err != nil {
		return
	}
	return
}

const roomsSchema = `
CREATE SEQUENCE IF NOT EXISTS room_nid_seq;
CREATE TABLE IF NOT EXISTS rooms (
    -- Local numeric ID for the room.
    room_nid BIGINT PRIMARY KEY DEFAULT nextval('room_nid_seq'),
    -- Textual ID for the room.
    room_id TEXT NOT NULL CONSTRAINT room_id_unique UNIQUE
);
`

// Same as insertEventTypeNIDSQL
const insertRoomNIDSQL = "" +
	"INSERT INTO rooms (room_id) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT room_id_unique" +
	" DO UPDATE SET room_id = $1" +
	" RETURNING (room_nid)"

const selectRoomNIDSQL = "" +
	"SELECT room_nid FROM rooms WHERE room_id = $1"

func (s *statements) insertRoomNID(roomID string) (roomNID int64, err error) {
	err = s.insertRoomNIDStmt.QueryRow(roomID).Scan(&roomNID)
	return
}

func (s *statements) selectRoomNID(roomID string) (roomNID int64, err error) {
	err = s.selectRoomNIDStmt.QueryRow(roomID).Scan(&roomNID)
	return
}

const eventsSchema = `
-- The events table holds metadata for each event, the actual JSON is stored
-- separately to keep the size of the rows small.
CREATE SEQUENCE IF NOT EXISTS event_nid_seq;
CREATE TABLE IF NOT EXISTS events (
    -- Local numeric ID for the event.
    event_nid BIGINT PRIMARY KEY DEFAULT nextval('event_nid_seq'),
    -- Local numeric ID for the room the event is in.
    -- This is never 0.
    room_nid BIGINT NOT NULL,
    -- Local numeric ID for the type of the event.
    -- This is never 0.
    event_type_nid BIGINT NOT NULL,
    -- Local numeric ID for the state_key of the event
    -- This is 0 if the event is not a state event.
    event_state_key_nid BIGINT NOT NULL,
	-- Local numeric ID for the state at the event.
    -- This is 0 if we don't know the state at the event.
    -- If the state is not 0 this this event is part of the contiguous
    -- part of the event graph
    -- Since many different events can have the same state we store the
    -- state into a separate state table and refer to it by numeric ID.
    state_nid bigint NOT NULL DEFAULT 0
    -- The textual event id.
    -- Used to lookup the numeric ID when processing requests.
    -- Needed for state resolution.
    -- An event may only appear in this table once.
    event_id TEXT NOT NULL CONSTRAINT event_id_unique UNIQUE,
    -- The sha256 reference hash for the event.
    -- Needed for setting reference hashes when sending new events.
    reference_sha256 BYTEA NOT NULL,
    -- A list of numeric IDs for events that can authenticate this event.
    auth_event_nids BIGINT[] NOT NULL,
);
`

const insertEventSQL = "" +
	"INSERT INTO events (room_nid, event_type_nid, event_state_key_nid, event_id, reference_sha256, auth_event_nids)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT ON CONSTRAINT event_id_unique" +
	" DO UPDATE SET event_id = $1" +
	" RETURNING event_nid, state_nid"

// Bulk lookup of events by string ID.
// Sort by the numeric IDs for event type and state key.
// This means we can use binary search to lookup entries by type and state key.
const bulkSelectStateEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid FROM events" +
	" WHERE event_id = ANY($1)" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

const bulkSelectStateAtEventByIDSQL = "" +
	"SELECT event_type_nid, event_state_key_nid, event_nid, state_nid FROM events" +
	" WHERE event_id = ANY($1)" +
	" ORDER BY event_type_nid, event_state_key_nid ASC"

const updateEventStateSQL = "" +
	"UPDATE events SET state_nid = $2 WHERE event_nid = $1"

func (s *statements) prepareEvents(db *sql.DB) (err error) {
	_, err = db.Exec(eventsSchema)
	if err != nil {
		return
	}
	if s.insertEventStmt, err = db.Prepare(insertEventSQL); err != nil {
		return
	}
	if s.bulkSelectStateEventByIDStmt, err = db.Prepare(bulkSelectStateEventByIDSQL); err != nil {
		return
	}
	if s.bulkSelectStateAtEventByIDStmt, err = db.Prepare(bulkSelectStateAtEventByIDSQL); err != nil {
		return
	}
	if s.updateEventStateStmt, err = db.Prepare(updateEventStateSQL); err != nil {
		return
	}
	return
}

func (s *statements) insertEvent(
	roomNID, eventTypeNID, eventStateKeyNID int64,
	eventID string,
	referenceSHA256 []byte,
	authEventNIDs []int64,
) (eventNID, stateNID int64, err error) {
	err = s.insertEventStmt.QueryRow(
		roomNID, eventTypeNID, eventStateKeyNID, eventID, referenceSHA256,
		pq.Int64Array(authEventNIDs),
	).Scan(&eventNID, &stateNID)
	return
}

func (s *statements) bulkSelectStateEventByID(eventIDs []string) ([]types.StateEntry, error) {
	rows, err := s.bulkSelectStateEventByIDStmt.Query(pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// We know that we will only get as many results as event IDs
	// because of the unique constraint on event IDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than IDs so we adjust the length of the slice before returning it.
	results := make([]types.StateEntry, len(eventIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(
			&result.EventNID,
			&result.EventTypeNID,
			&result.EventStateKeyNID,
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
		return nil, fmt.Errorf("storage: state event IDs missing from the database (%d != %d)", i, len(eventIDs))
	}
	return results, err
}

func (s *statements) bulkSelectStateAtEventByID(eventIDs []string) ([]types.StateAtEvent, error) {
	rows, err := s.bulkSelectStateAtEventByIDStmt.Query(pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]types.StateAtEvent, len(eventIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(
			&result.EventNID,
			&result.EventTypeNID,
			&result.EventStateKeyNID,
			&result.BeforeStateNID,
		); err != nil {
			return nil, err
		}
		if result.BeforeStateNID == 0 {
			return nil, fmt.Errorf("storage: missing state for event NID %d", result.EventNID)
		}
	}
	if i != len(eventIDs) {
		return nil, fmt.Errorf("storage: event IDs missing from the database (%d != %d)", i, len(eventIDs))
	}
	return results, err
}

func (s *statements) updateEventState(eventNID, stateNID int64) error {
	_, err := s.updateEventStateStmt.Exec(eventNID, stateNID)
	return err
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

func (s *statements) insertEventJSON(eventNID int64, eventJSON []byte) error {
	_, err := s.insertEventJSONStmt.Exec(eventNID, eventJSON)
	return err
}

type eventJSONPair struct {
	EventNID  int64
	EventJSON []byte
}

func (s *statements) bulkSelectEventJSON(eventNIDs []int64) ([]eventJSONPair, error) {
	rows, err := s.bulkSelectEventJSONStmt.Query(pq.Int64Array(eventNIDs))
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
		if err := rows.Scan(&results[i].EventNID, &results[i].EventJSON); err != nil {
			return nil, err
		}
	}
	return results[:i], nil
}

const stateSchema = `
-- The state of a room before an event.
-- Stored as a list of state_data entries stored in a separate table.
-- The actual state is constructed by combining all the state_data entries
-- referenced by state_data_nids together. If the same state key tuple appears
-- multiple times then the entry from the later state_data clobbers the earlier
-- entries.
-- This encoding format allows us to implement a delta encoding which is useful
-- because room state tends to accumulate small changes over time. Although if
-- the list of deltas becomes too long it becomes more efficient to encode
-- the full state under single state_data_nid.
CREATE SEQUENCE IF NOT EXISTS state_nid_seq;
CREATE TABLE IF NOT EXISTS state (
    -- Local numeric ID for the state.
    state_nid bigint PRIMARY KEY DEFAULT nextval('state_nid_seq'),
    -- Local numeric ID of the room this state is for.
    -- Unused in normal operation, but useful for background work or ad-hoc debugging.
    room_nid bigint NOT NULL,
    -- List of state_data_nids, stored sorted by state_data_nid.
    state_data_nids bigint[] NOT NULL
);
`

const insertStateSQL = "" +
	"INSERT INTO state (room_nid, state_data_nids)" +
	" VALUES ($1, $2)" +
	" RETURNING state_nid"

const bulkSelectStateDataNIDsSQL = "" +
	"SELECT state_nid, state_data_nids FROM state" +
	" WHERE state_nid = ANY($1) ORDER BY state_nid"

func (s *statements) prepareState(db *sql.DB) (err error) {
	_, err = db.Exec(stateSchema)
	if err != nil {
		return
	}
	if s.insertStateStmt, err = db.Prepare(insertStateSQL); err != nil {
		return
	}
	if s.bulkSelectStateDataNIDsStmt, err = db.Prepare(bulkSelectStateDataNIDsSQL); err != nil {
		return
	}
	return
}

func (s *statements) insertState(roomNID int64, stateDataNIDs []int64) (stateNID int64, err error) {
	err = s.insertStateStmt.QueryRow(roomNID, pq.Int64Array(stateDataNIDs)).Scan(&stateNID)
	return
}

func (s *statements) bulkSelectStateDataNIDs(stateNIDs []int64) ([]types.StateDataNIDList, error) {
	rows, err := s.bulkSelectStateDataNIDsStmt.Query(pq.Int64Array(stateNIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]types.StateDataNIDList, len(stateNIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		var stateDataNids pq.Int64Array
		if err := rows.Scan(&result.StateNID, &stateDataNids); err != nil {
			return nil, err
		}
		result.StateDataNIDs = stateDataNids
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
-- to read the state for a given state_data_nid ordered by (type, state_key)
-- which in turn makes it easier to merge state data blocks.
CREATE SEQUENCE IF NOT EXISTS state_data_nid_seq;
CREATE TABLE IF NOT EXISTS state_data (
    -- Local numeric ID for this state data.
    state_data_nid bigint NOT NULL,
    event_type_nid bigint NOT NULL,
    event_state_key_nid bigint NOT NULL,
    event_nid bigint NOT NULL,
    UNIQUE (state_data_nid, event_type_nid, event_state_key_nid)
);
`

const insertStateDataSQL = "" +
	"INSERT INTO state_data (state_data_nid, event_type_nid, event_state_key_nid, event_nid)" +
	" VALUES ($1, $2, $3, $4)"

const selectNextStateDataNIDSQL = "" +
	"SELECT nextval('state_data_nid_seq')"

const bulkSelectStateDataEntriesSQL = "" +
	"SELECT state_data_nid, event_type_nid, event_state_key_nid, event_nid" +
	" FROM state_data WHERE state_data_nid = ANY($1)" +
	" ORDER BY state_data_nid, event_type_nid, event_state_key_nid"

func (s *statements) prepareStateData(db *sql.DB) (err error) {
	_, err = db.Exec(stateDataSchema)
	if err != nil {
		return
	}
	if s.insertStateDataStmt, err = db.Prepare(insertStateDataSQL); err != nil {
		return
	}
	if s.selectNextStateDataNIDStmt, err = db.Prepare(selectNextStateDataNIDSQL); err != nil {
		return
	}

	if s.bulkSelectStateDataEntriesStmt, err = db.Prepare(bulkSelectStateDataEntriesSQL); err != nil {
		return
	}
	return
}

func (s *statements) bulkInsertStateData(stateDataNID int64, entries []types.StateEntry) error {
	for _, entry := range entries {
		_, err := s.insertStateDataStmt.Exec(
			stateDataNID,
			entry.EventTypeNID,
			entry.EventStateKeyNID,
			entry.EventNID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *statements) selectNextStateDataNID() (stateDataNID int64, err error) {
	err = s.selectNextStateDataNIDStmt.QueryRow().Scan(&stateDataNID)
	return
}

func (s *statements) bulkSelectStateDataEntries(stateDataNIDs []int64) ([]types.StateEntryList, error) {
	rows, err := s.bulkSelectStateDataEntriesStmt.Query(pq.Int64Array(stateDataNIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]types.StateEntryList, len(stateDataNIDs))
	// current is a pointer to the StateEntryList to append the state entries to.
	var current *types.StateEntryList
	i := 0
	for rows.Next() {
		var stateDataNID int64
		var entry types.StateEntry
		if err := rows.Scan(
			&stateDataNID,
			&entry.EventTypeNID, &entry.EventStateKeyNID, &entry.EventNID,
		); err != nil {
			return nil, err
		}
		if current == nil || stateDataNID != current.StateDataNID {
			// The state entry row is for a different state data block to the current one.
			// So we start appending to the next entry in the list.
			current = &results[i]
			current.StateDataNID = stateDataNID
			i++
		}
		current.StateEntries = append(current.StateEntries, entry)
	}
	if i != len(stateDataNIDs) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", i, len(stateDataNIDs))
	}
	return results, nil
}
