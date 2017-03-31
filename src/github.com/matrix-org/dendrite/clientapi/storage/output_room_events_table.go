package storage

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
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
`

const insertEventSQL = "" +
	"INSERT INTO output_room_events (room_id, event_id, event_json, add_state_ids, remove_state_ids) VALUES ($1, $2, $3, $4, $5)"

const selectEventsSQL = "" +
	"SELECT event_json FROM output_room_events WHERE event_id = ANY($1)"

type outputRoomEventsStatements struct {
	insertEventStmt  *sql.Stmt
	selectEventsStmt *sql.Stmt
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
	return
}

// InsertEvent into the output_room_events table. addState and removeState are an optional list of state event IDs.
func (s *outputRoomEventsStatements) InsertEvent(txn *sql.Tx, event *gomatrixserverlib.Event, addState, removeState []string) error {
	_, err := txn.Stmt(s.insertEventStmt).Exec(
		event.RoomID(), event.EventID(), event.JSON(), pq.StringArray(addState), pq.StringArray(removeState),
	)
	return err
}

// Events returns the events for the given event IDs. Returns an error if any one of the event IDs given are missing
// from the database.
func (s *outputRoomEventsStatements) Events(txn *sql.Tx, eventIDs []string) ([]gomatrixserverlib.Event, error) {
	rows, err := txn.Stmt(s.selectEventsStmt).Query(pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]gomatrixserverlib.Event, len(eventIDs))
	i := 0
	for ; rows.Next(); i++ {
		var eventBytes []byte
		if err := rows.Scan(&eventBytes); err != nil {
			return nil, err
		}
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false)
		if err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	if i != len(eventIDs) {
		return nil, fmt.Errorf("failed to map all event IDs to events: (%d != %d)", i, len(eventIDs))
	}
	return result, nil
}
