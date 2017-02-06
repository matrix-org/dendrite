package storage

import (
	"database/sql"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type statements struct {
	selectPartitionOffsetsStmt *sql.Stmt
	upsertPartitionOffsetStmt  *sql.Stmt
	insertEventTypeNIDStmt     *sql.Stmt
	selectEventTypeNIDStmt     *sql.Stmt
	insertEventStateKeyNIDStmt *sql.Stmt
	selectEventStateKeyNIDStmt *sql.Stmt
	insertRoomNIDStmt          *sql.Stmt
	selectRoomNIDStmt          *sql.Stmt
	insertEventStmt            *sql.Stmt
	insertEventJSONStmt        *sql.Stmt
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
	if err == sql.ErrNoRows {
		eventTypeNID = 0
		err = nil
	}
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
CREATE SEQUENCE IF NOT EXISTS event_state_key_nid_seq START 65536;
CREATE TABLE IF NOT EXISTS event_state_keys (
    -- Local numeric ID for the state key.
    event_state_key_nid BIGINT PRIMARY KEY DEFAULT nextval('event_state_key_nid_seq'),
    event_state_key TEXT NOT NULL CONSTRAINT event_state_key_unique UNIQUE
);
INSERT INTO event_state_keys (event_state_key_nid, event_state_key) VALUES
    (1, '') ON CONFLICT DO NOTHING;
`

const insertEventStateKeyNIDSQL = "" +
	"INSERT INTO event_state_keys (event_state_key) VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT event_state_key_unique" +
	" DO UPDATE SET event_state_key = $1" +
	" RETURNING (event_state_key_nid)"

const selectEventStateKeyNIDSQL = "" +
	"SELECT event_state_key_nid FROM event_state_keys WHERE event_state_key = $1"

func (s *statements) insertEventStateKeyNID(eventStateKey string) (eventStateKeyNID int64, err error) {
	err = s.insertEventStateKeyNIDStmt.QueryRow(eventStateKey).Scan(&eventStateKeyNID)
	return
}

func (s *statements) selectEventStateKeyNID(eventStateKey string) (eventStateKeyNID int64, err error) {
	err = s.selectEventStateKeyNIDStmt.QueryRow(eventStateKey).Scan(&eventStateKeyNID)
	if err == sql.ErrNoRows {
		eventStateKeyNID = 0
		err = nil
	}
	return
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
	if err == sql.ErrNoRows {
		roomNID = 0
		err = nil
	}
	return
}

const eventsSchema = `
-- The events table holds metadata for each event, the actual JSON is stored
-- separately to keep the size of the rows small.
CREATE SEQUENCE IF NOT EXISTS event_nid_seq;
CREATE TABLE IF NOT EXISTS events (
    -- Local numeric ID for the event.
    event_nid bigint PRIMARY KEY DEFAULT nextval('event_nid_seq'),
    -- Local numeric ID for the room the event is in.
    room_nid bigint NOT NULL,
	-- Local numeric ID for the type of the event.
    event_type_nid bigint NOT NULL,
    -- Local numeric ID for the state_key of the event
	-- This is 0 if the event is not a state event.
    event_state_key_nid bigint NOT NULL,
    -- The textual event id.
    -- Used to lookup the numeric ID when processing requests.
    -- Needed for state resolution.
    -- An event may only appear in this table once.
    event_id text NOT NULL CONSTRAINT event_id_unique UNIQUE,
    -- The sha256 reference hash for the event.
    -- Needed for setting reference hashes when sending new events.
    reference_sha256 bytea NOT NULL
);
`

const insertEventSQL = "" +
	"INSERT INTO events (room_nid, event_type_nid, event_state_key_nid, event_id, reference_sha256)" +
	" VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT ON CONSTRAINT event_id_unique" +
	" DO UPDATE SET event_id = $1" +
	" RETURNING event_nid"

func (s *statements) prepareEvents(db *sql.DB) (err error) {
	_, err = db.Exec(eventsSchema)
	if err != nil {
		return
	}
	if s.insertEventStmt, err = db.Prepare(insertEventSQL); err != nil {
		return
	}
	return
}

func (s *statements) insertEvent(
	roomNID, eventTypeNID, eventStateKeyNID int64,
	eventID string,
	referenceSHA256 []byte,
) (eventNID int64, err error) {
	err = s.insertEventStmt.QueryRow(
		roomNID, eventTypeNID, eventStateKeyNID, eventID, referenceSHA256,
	).Scan(&eventNID)
	return
}

func (s *statements) prepareEventJSON(db *sql.DB) (err error) {
	_, err = db.Exec(eventJSONSchema)
	if err != nil {
		return
	}
	if s.insertEventJSONStmt, err = db.Prepare(insertEventJSONSQL); err != nil {
		return
	}
	return
}

const eventJSONSchema = `
-- Stores the JSON for each event. This kept separate from the main events
-- table to keep the rows in the main events table small.
CREATE TABLE IF NOT EXISTS event_json (
    -- Local numeric ID for the event.
    event_nid bigint NOT NULL PRIMARY KEY,
    -- The JSON for the event.
    -- Stored as TEXT because this should be valid UTF-8.
    -- Not stored as a JSONB because we always just pull the entire event.
    -- TODO: Should we be compressing the events with Snappy or DEFLATE?
    event_json text NOT NULL
);
`

const insertEventJSONSQL = "" +
	"INSERT INTO event_json (event_nid, event_json) VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

func (s *statements) insertEventJSON(eventNID int64, eventJSON []byte) error {
	_, err := s.insertEventJSONStmt.Exec(eventNID, eventJSON)
	return err
}
