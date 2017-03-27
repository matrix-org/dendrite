package storage

import (
	"database/sql"

	"github.com/lib/pq"
)

const outputRoomEventsSchema = `
-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS output_room_events (
    -- An incrementing ID which denotes the position in the log that this event resides at.
    -- NB: 'serial' makes no guarantees to increment by 1 every time, only that it increments.
    --     This isn't a problem for us since we just want to order by this field.
    id BIGSERIAL PRIMARY KEY,
    -- The 'room_id' key for the event.
    room_id TEXT NOT NULL,
    -- The JSON for the event. Stored as TEXT because this should be valid UTF-8.
    event_json TEXT NOT NULL,
    -- A list of event IDs which represent a delta of added/removed room state. May be NULL
    -- if no state has been added/removed.
    add_state_ids TEXT[],
    remove_state_ids TEXT[]
);
`

const insertEventSQL = "" +
	"INSERT INTO output_room_events (room_id, event_json, add_state_ids, remove_state_ids) VALUES ($1, $2, $3, $4)"

type outputRoomEventsStatements struct {
	insertEventStmt *sql.Stmt
}

func (s *outputRoomEventsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(outputRoomEventsSchema)
	if err != nil {
		return
	}
	if s.insertEventStmt, err = db.Prepare(insertEventSQL); err != nil {
		return
	}
	return
}

// InsertEvent into the output_room_events table.
func (s *outputRoomEventsStatements) InsertEvent(roomID string, eventJSON []byte, addState, removeState []string) error {
	_, err := s.insertEventStmt.Exec(roomID, eventJSON, pq.StringArray(addState), pq.StringArray(removeState))
	return err
}
