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
-- for querying membership states of users
CREATE INDEX IF NOT EXISTS membership_idx ON current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
`

const upsertRoomStateSQL = "" +
	"INSERT INTO current_room_state (room_id, event_id, type, state_key, event_json, membership) VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT ON CONSTRAINT room_state_unique" +
	" DO UPDATE SET event_id = $2, event_json = $5, membership = $6"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM current_room_state WHERE event_id = $1"

const selectRoomIDsWithMembershipSQL = "" +
	"SELECT room_id FROM current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"

const selectCurrentStateSQL = "" +
	"SELECT event_json FROM current_room_state WHERE room_id = $1"

type currentRoomStateStatements struct {
	upsertRoomStateStmt             *sql.Stmt
	deleteRoomStateByEventIDStmt    *sql.Stmt
	selectRoomIDsWithMembershipStmt *sql.Stmt
	selectCurrentStateStmt          *sql.Stmt
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
	if s.selectRoomIDsWithMembershipStmt, err = db.Prepare(selectRoomIDsWithMembershipSQL); err != nil {
		return
	}
	if s.selectCurrentStateStmt, err = db.Prepare(selectCurrentStateSQL); err != nil {
		return
	}
	return
}

// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
func (s *currentRoomStateStatements) SelectRoomIDsWithMembership(txn *sql.Tx, userID, membership string) ([]string, error) {
	rows, err := txn.Stmt(s.selectRoomIDsWithMembershipStmt).Query(userID, membership)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var roomID string
		if err := rows.Scan(&roomID); err != nil {
			return nil, err
		}
		result = append(result, roomID)
	}
	return result, nil
}

// CurrentState returns all the current state events for the given room.
func (s *currentRoomStateStatements) CurrentState(txn *sql.Tx, roomID string) ([]gomatrixserverlib.Event, error) {
	rows, err := txn.Stmt(s.selectCurrentStateStmt).Query(roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
