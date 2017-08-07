// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
)

const currentRoomStateSchema = `
-- Stores the current room state for every room.
CREATE TABLE IF NOT EXISTS syncapi_current_room_state (
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
    -- The serial ID of the output_room_events table when this event became
    -- part of the current state of the room.
    added_at BIGINT,
    -- Clobber based on 3-uple of room_id, type and state_key
    CONSTRAINT syncapi_room_state_unique UNIQUE (room_id, type, state_key)
);
-- for event deletion
CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_current_room_state(event_id);
-- for querying membership states of users
CREATE INDEX IF NOT EXISTS syncapi_membership_idx ON syncapi_current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
`

const upsertRoomStateSQL = "" +
	"INSERT INTO syncapi_current_room_state (room_id, event_id, type, state_key, event_json, membership, added_at)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7)" +
	" ON CONFLICT ON CONSTRAINT syncapi_room_state_unique" +
	" DO UPDATE SET event_id = $2, event_json = $5, membership = $6, added_at = $7"

const deleteRoomStateByEventIDSQL = "" +
	"DELETE FROM syncapi_current_room_state WHERE event_id = $1"

const selectRoomIDsWithMembershipSQL = "" +
	"SELECT room_id FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"

const selectCurrentStateSQL = "" +
	"SELECT event_json FROM syncapi_current_room_state WHERE room_id = $1"

const selectJoinedUsersSQL = "" +
	"SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join'"

const selectStateEventSQL = "" +
	"SELECT event_json FROM syncapi_current_room_state WHERE type = $1 AND room_id = $2 AND state_key = $3"

const selectEventsWithEventIDsSQL = "" +
	"SELECT added_at, event_json FROM syncapi_current_room_state WHERE event_id = ANY($1)"

type currentRoomStateStatements struct {
	upsertRoomStateStmt             *sql.Stmt
	deleteRoomStateByEventIDStmt    *sql.Stmt
	selectRoomIDsWithMembershipStmt *sql.Stmt
	selectCurrentStateStmt          *sql.Stmt
	selectJoinedUsersStmt           *sql.Stmt
	selectEventsWithEventIDsStmt    *sql.Stmt
	selectStateEventStmt            *sql.Stmt
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
	if s.selectJoinedUsersStmt, err = db.Prepare(selectJoinedUsersSQL); err != nil {
		return
	}
	if s.selectEventsWithEventIDsStmt, err = db.Prepare(selectEventsWithEventIDsSQL); err != nil {
		return
	}
	if s.selectStateEventStmt, err = db.Prepare(selectStateEventSQL); err != nil {
		return
	}
	return
}

// JoinedMemberLists returns a map of room ID to a list of joined user IDs.
func (s *currentRoomStateStatements) selectJoinedUsers() (map[string][]string, error) {
	rows, err := s.selectJoinedUsersStmt.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var roomID string
		var userID string
		if err := rows.Scan(&roomID, &userID); err != nil {
			return nil, err
		}
		users := result[roomID]
		users = append(users, userID)
		result[roomID] = users
	}
	return result, nil
}

// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
func (s *currentRoomStateStatements) selectRoomIDsWithMembership(txn *sql.Tx, userID, membership string) ([]string, error) {
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
func (s *currentRoomStateStatements) selectCurrentState(txn *sql.Tx, roomID string) ([]gomatrixserverlib.Event, error) {
	rows, err := txn.Stmt(s.selectCurrentStateStmt).Query(roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return rowsToEvents(rows)
}

func (s *currentRoomStateStatements) deleteRoomStateByEventID(txn *sql.Tx, eventID string) error {
	_, err := txn.Stmt(s.deleteRoomStateByEventIDStmt).Exec(eventID)
	return err
}

func (s *currentRoomStateStatements) upsertRoomState(
	txn *sql.Tx, event gomatrixserverlib.Event, membership *string, addedAt int64,
) error {
	_, err := txn.Stmt(s.upsertRoomStateStmt).Exec(
		event.RoomID(), event.EventID(), event.Type(), *event.StateKey(), event.JSON(), membership, addedAt,
	)
	return err
}

func (s *currentRoomStateStatements) selectEventsWithEventIDs(txn *sql.Tx, eventIDs []string) ([]streamEvent, error) {
	rows, err := txn.Stmt(s.selectEventsWithEventIDsStmt).Query(pq.StringArray(eventIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rowsToStreamEvents(rows)
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

func (s *currentRoomStateStatements) selectStateEvent(evType string, roomID string, stateKey string) (*gomatrixserverlib.Event, error) {
	var res []byte
	if err := s.selectStateEventStmt.QueryRow(evType, roomID, stateKey).Scan(&res); err == sql.ErrNoRows {
		return nil, nil
	}
	ev, err := gomatrixserverlib.NewEventFromTrustedJSON(res, false)
	return &ev, err
}
