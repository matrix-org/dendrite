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

    -- A user can only be member of a room once
    PRIMARY KEY (localpart, room_id)
);
`

const insertMembershipSQL = "" +
	"INSERT INTO memberships(localpart, room_id, event_id) VALUES ($1, $2, $3)"

const selectMembershipSQL = "" +
	"SELECT * from memberships WHERE localpart = $1 AND room_id = $2"

const selectMembershipByEventIDSQL = "" +
	"SELECT localpart, room_id FROM memberships WHERE event_id = $1"

const selectMembershipsByLocalpartSQL = "" +
	"SELECT room_id FROM memberships WHERE localpart = $1"

const deleteMembershipSQL = "" +
	"DELETE FROM memberships WHERE localpart = $1 AND room_id = $2"

const deleteMembershipByEventIDSQL = "" +
	"DELETE FROM memberships WHERE event_id = $1"

type membershipStatements struct {
	deleteMembershipByEventIDStmt    *sql.Stmt
	deleteMembershipStmt             *sql.Stmt
	insertMembershipStmt             *sql.Stmt
	selectMembershipByEventIDStmt    *sql.Stmt
	selectMembershipsByLocalpartStmt *sql.Stmt
	selectMembershipStmt             *sql.Stmt
}

func (s *membershipStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(membershipSchema)
	if err != nil {
		return
	}
	if s.deleteMembershipByEventIDStmt, err = db.Prepare(deleteMembershipByEventIDSQL); err != nil {
		return
	}
	if s.deleteMembershipStmt, err = db.Prepare(deleteMembershipSQL); err != nil {
		return
	}
	if s.insertMembershipStmt, err = db.Prepare(insertMembershipSQL); err != nil {
		return
	}
	if s.selectMembershipByEventIDStmt, err = db.Prepare(selectMembershipByEventIDSQL); err != nil {
		return
	}
	if s.selectMembershipsByLocalpartStmt, err = db.Prepare(selectMembershipsByLocalpartSQL); err != nil {
		return
	}
	if s.selectMembershipStmt, err = db.Prepare(selectMembershipSQL); err != nil {
		return
	}
	return
}

func (s *membershipStatements) insertMembership(localpart string, roomID string, eventID string) (err error) {
	_, err = s.insertMembershipStmt.Exec(localpart, roomID, eventID)
	return
}

func (s *membershipStatements) deleteMembership(localpart string, roomID string) (err error) {
	_, err = s.deleteMembershipStmt.Exec(localpart, roomID)
	return
}

func (s *membershipStatements) deleteMembershipByEventID(eventID string) (err error) {
	_, err = s.deleteMembershipByEventIDStmt.Exec(eventID)
	return
}

func (s *membershipStatements) selectMembershipByEventID(eventID string) (localpart string, roomID string, err error) {
	err = s.selectMembershipByEventIDStmt.QueryRow(eventID).Scan(&localpart, &roomID)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return
}
