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
INSERT INTO event_types (event_type_nid, event_type) VALUES (
    (1, 'm.room.create'),
    (2, 'm.room.power_levels'),
    (3, 'm.room.join_rules'),
    (4, 'm.room.third_party_invite'),
    (5, 'm.room.member'),
    (6, 'm.room.redaction'),
    (7, 'm.room.history_visibility'),
) ON CONFLICT DO NOTHING;
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
	if err == sql.ErrNoRows {
		eventTypeNID = 0
		err = nil
	}
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
INSERT INTO event_state_keys (event_state_key_nid, event_state_key) VALUES (
    (1, '')
) ON CONFLICT DO NOTHING;
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
	if err == sql.ErrNoRows {
		eventStateKeyNID = 0
		err = nil
	}
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
