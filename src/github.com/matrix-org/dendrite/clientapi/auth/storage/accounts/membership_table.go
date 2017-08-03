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

package accounts

import (
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

const membershipSchema = `
-- Stores data about users memberships to rooms.
CREATE TABLE IF NOT EXISTS memberships (
    -- The Matrix user ID localpart for the member
    localpart TEXT NOT NULL,
    -- The room this user is a member of
    room_id TEXT NOT NULL,
    -- The ID of the join membership event
    event_id TEXT NOT NULL,
	-- Is the user still in the room? We use a boolean instead of just deleting
	-- the row to know if the user has been in the room at some point
	-- If set to false, event_id would be the event ID of the leave/kick/ban
	-- membership event
	still_in_room BOOLEAN NOT NULL,

    -- A user can only be member of a room once
    PRIMARY KEY (localpart, room_id)
);

-- Use index to process deletion by ID more efficiently
CREATE UNIQUE INDEX IF NOT EXISTS membership_event_id ON memberships(event_id);
`

const insertMembershipSQL = `
	INSERT INTO memberships(localpart, room_id, event_id, still_in_room)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (localpart, room_id) DO
	UPDATE SET event_id = EXCLUDED.event_id, still_in_room = EXCLUDED.still_in_room
`

const selectMembershipSQL = "" +
	"SELECT * from memberships WHERE localpart = $1 AND room_id = $2"

const selectMembershipsByLocalpartSQL = "" +
	"SELECT room_id, event_id FROM memberships WHERE localpart = $1 AND still_in_room = true"

const selectMembershipsByRoomIDSQL = "" +
	"SELECT localpart, event_id FROM memberships WHERE room_id = $1 AND still_in_room = true"

const deleteMembershipsByEventIDsSQL = "" +
	"DELETE FROM memberships WHERE event_id = ANY($1)"

const updateMembershipByEventIDSQL = "" +
	"UPDATE memberships SET event_id = $2, still_in_room = $3 WHERE event_id = $1"

type membershipStatements struct {
	deleteMembershipsByEventIDsStmt  *sql.Stmt
	insertMembershipStmt             *sql.Stmt
	selectMembershipByEventIDStmt    *sql.Stmt
	selectMembershipsByLocalpartStmt *sql.Stmt
	selectMembershipsByRoomIDStmt    *sql.Stmt
	updateMembershipByEventIDStmt    *sql.Stmt
}

func (s *membershipStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(membershipSchema)
	if err != nil {
		return
	}
	if s.deleteMembershipsByEventIDsStmt, err = db.Prepare(deleteMembershipsByEventIDsSQL); err != nil {
		return
	}
	if s.insertMembershipStmt, err = db.Prepare(insertMembershipSQL); err != nil {
		return
	}
	if s.selectMembershipsByLocalpartStmt, err = db.Prepare(selectMembershipsByLocalpartSQL); err != nil {
		return
	}
	if s.selectMembershipsByRoomIDStmt, err = db.Prepare(selectMembershipsByRoomIDSQL); err != nil {
		return
	}
	if s.updateMembershipByEventIDStmt, err = db.Prepare(updateMembershipByEventIDSQL); err != nil {
		return
	}
	return
}

func (s *membershipStatements) insertMembership(localpart string, roomID string, eventID string, stillInRoom bool, txn *sql.Tx) (err error) {
	_, err = txn.Stmt(s.insertMembershipStmt).Exec(localpart, roomID, eventID, stillInRoom)
	return
}

func (s *membershipStatements) deleteMembershipsByEventIDs(eventIDs []string, txn *sql.Tx) (err error) {
	_, err = txn.Stmt(s.deleteMembershipsByEventIDsStmt).Exec(pq.StringArray(eventIDs))
	return
}

func (s *membershipStatements) selectMembershipsByLocalpart(localpart string) (memberships []authtypes.Membership, err error) {
	rows, err := s.selectMembershipsByLocalpartStmt.Query(localpart)
	if err != nil {
		return
	}

	memberships = []authtypes.Membership{}

	defer rows.Close()
	for rows.Next() {
		var m authtypes.Membership
		m.Localpart = localpart
		if err := rows.Scan(&m.RoomID, &m.EventID); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}

	return
}

func (s *membershipStatements) selectMembershipsByRoomID(roomID string) (memberships []authtypes.Membership, err error) {
	rows, err := s.selectMembershipsByRoomIDStmt.Query(roomID)
	if err != nil {
		return
	}

	memberships = []authtypes.Membership{}

	defer rows.Close()
	for rows.Next() {
		var m authtypes.Membership
		m.RoomID = roomID
		if err := rows.Scan(&m.Localpart, &m.EventID); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}

	return
}

func (s *membershipStatements) updateMembershipByEventID(oldEventID string, newEventID string) (err error) {
	_, err = s.updateMembershipByEventIDStmt.Exec(oldEventID, newEventID)
	return
}
