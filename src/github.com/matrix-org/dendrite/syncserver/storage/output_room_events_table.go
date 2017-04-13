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
-- for event selection
CREATE UNIQUE INDEX IF NOT EXISTS event_id_idx ON output_room_events(event_id);
`

const insertEventSQL = "" +
	"INSERT INTO output_room_events (room_id, event_id, event_json, add_state_ids, remove_state_ids) VALUES ($1, $2, $3, $4, $5) RETURNING id"

const selectEventsSQL = "" +
	"SELECT event_json FROM output_room_events WHERE event_id = ANY($1)"

const selectEventsInRangeSQL = "" +
	"SELECT event_json FROM output_room_events WHERE id > $1 AND id <= $2"

const selectRecentEventsSQL = "" +
	"SELECT event_json FROM output_room_events WHERE room_id = $1 ORDER BY id DESC LIMIT $2"

const selectMaxIDSQL = "" +
	"SELECT MAX(id) FROM output_room_events"

type outputRoomEventsStatements struct {
	insertEventStmt         *sql.Stmt
	selectEventsStmt        *sql.Stmt
	selectMaxIDStmt         *sql.Stmt
	selectEventsInRangeStmt *sql.Stmt
	selectRecentEventsStmt  *sql.Stmt
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
	if s.selectEventsInRangeStmt, err = db.Prepare(selectEventsInRangeSQL); err != nil {
		return
	}
	if s.selectRecentEventsStmt, err = db.Prepare(selectRecentEventsSQL); err != nil {
		return
	}
	return
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

// InRange returns all the events in the range between oldPos exclusive and newPos inclusive. Returns an empty array if
// there are no events between the provided range. Returns an error if events are missing in the range.
func (s *outputRoomEventsStatements) InRange(oldPos, newPos int64) ([]gomatrixserverlib.Event, error) {
	rows, err := s.selectEventsInRangeStmt.Query(oldPos, newPos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result, err := rowsToEvents(rows)
	if err != nil {
		return nil, err
	}
	// Expect one event per position, exclusive of old. eg old=3, new=5, expect 4,5 so 2 events.
	wantNum := int(newPos - oldPos)
	if len(result) != wantNum {
		return nil, fmt.Errorf("failed to map all positions to events: (got %d, wanted, %d)", len(result), wantNum)
	}
	return result, nil
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
func (s *outputRoomEventsStatements) RecentEventsInRoom(txn *sql.Tx, roomID string, limit int) ([]gomatrixserverlib.Event, error) {
	rows, err := s.selectRecentEventsStmt.Query(roomID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rowsToEvents(rows)
}

// Events returns the events for the given event IDs. Returns an error if any one of the event IDs given are missing
// from the database.
func (s *outputRoomEventsStatements) Events(txn *sql.Tx, eventIDs []string) ([]gomatrixserverlib.Event, error) {
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

func rowsToEvents(rows *sql.Rows) ([]gomatrixserverlib.Event, error) {
	var result []gomatrixserverlib.Event
	for rows.Next() {
		var eventBytes []byte
		if err := rows.Scan(&eventBytes); err != nil {
			return nil, err
		}
		// TODO: Handle redacted events
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON(eventBytes, false)
		if err != nil {
			return nil, err
		}
		result = append(result, ev)
	}
	return result, nil
}
