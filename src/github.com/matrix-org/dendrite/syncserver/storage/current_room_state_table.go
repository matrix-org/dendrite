package storage

import (
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/clientapi/events"
	"github.com/matrix-org/gomatrixserverlib"
)

const currentRoomStateSchema = `
-- Stores the current room state for every room.
CREATE TABLE IF NOT EXISTS current_room_state (
    -- The 'room_id' key for the state event.
    room_id TEXT NOT NULL,
    -- The state event ID
    event_id TEXT NOT NULL,
    -- The state event type e.g 'm.room.member'
    type TEXT NOT NULL,
    -- The state_key value for this state event e.g ''
    state_key TEXT NOT NULL,
    -- The JSON for the event. Stored as TEXT because this should be valid UTF-8.
    event_json TEXT NOT NULL,
    -- The 'content.membership' value if this event is an m.room.member event. For other
    -- events, this will be NULL.
    membership TEXT,
    -- Clobber based on 3-uple of room_id, type and state_key
    CONSTRAINT room_state_unique UNIQUE (room_id, type, state_key)
);
-- for event deletion
CREATE UNIQUE INDEX IF NOT EXISTS event_id_idx ON current_room_state(event_id);
`

const upsertRoomStateSQL = "" +
	"INSERT INTO current_room_state (room_id, event_id, type, state_key, event_json, membership) VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT ON CONSTRAINT room_state_unique" +
	" DO UPDATE SET event_id = $2, event_json = $5, membership = $6"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM current_room_state WHERE event_id = $1"

type currentRoomStateStatements struct {
	upsertRoomStateStmt          *sql.Stmt
	deleteRoomStateByEventIDStmt *sql.Stmt
}

func (s *currentRoomStateStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(currentRoomStateSchema)
	if err != nil {
		return
	}
	if s.upsertRoomStateStmt, err = db.Prepare(upsertRoomStateSQL); err != nil {
		return
	}
	if s.deleteRoomStateByEventIDStmt, err = db.Prepare(deleteRoomStateByEventIDSQL); err != nil {
		return
	}
	return
}

func (s *currentRoomStateStatements) UpdateRoomState(txn *sql.Tx, added []gomatrixserverlib.Event, removedEventIDs []string) error {
	// remove first, then add, as we do not ever delete state, but do replace state which is a remove followed by an add.
	for _, eventID := range removedEventIDs {
		_, err := txn.Stmt(s.deleteRoomStateByEventIDStmt).Exec(eventID)
		if err != nil {
			return err
		}
	}

	for _, event := range added {
		if event.StateKey() == nil {
			// ignore non state events
			continue
		}
		var membership *string
		if event.Type() == "m.room.member" {
			var memberContent events.MemberContent
			if err := json.Unmarshal(event.Content(), &memberContent); err != nil {
				return err
			}
			membership = &memberContent.Membership
		}
		_, err := txn.Stmt(s.upsertRoomStateStmt).Exec(
			event.RoomID(), event.EventID(), event.Type(), *event.StateKey(), event.JSON(), membership,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
