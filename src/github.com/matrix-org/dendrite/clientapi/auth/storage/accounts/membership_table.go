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
	"fmt"
)

const membershipSchema = `
-- Stores data about accounts profiles.
CREATE TABLE IF NOT EXISTS memberships (
    -- The Matrix user ID localpart for the member
    localpart TEXT NOT NULL,
    -- The room this user is a member of
    room_id TEXT NOT NULL,

	-- A user can only be member of a room once
	PRIMARY KEY (localpart, room_id)
);
`

const insertMembershipSQL = "" +
	"INSERT INTO memberships(localpart, room_id) VALUES ($1, $2)"

const selectMembershipSQL = "" +
	"SELECT * from memberships WHERE localpart = $1 AND room_id = $2"

const selectMembershipsByLocalpartSQL = "" +
	"SELECT room_id FROM memberships WHERE localpart = $1"

const deleteMembershipSQL = "" +
	"DELETE FROM memberships WHERE localpart = $1 AND room_id = $2"

type membershipStatements struct {
	deleteMembershipStmt             *sql.Stmt
	insertMembershipStmt             *sql.Stmt
	selectMembershipsByLocalpartStmt *sql.Stmt
	selectMembershipStmt             *sql.Stmt
}

func (s *membershipStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(membershipSchema)
	if err != nil {
		return
	}
	if s.deleteMembershipStmt, err = db.Prepare(deleteMembershipSQL); err != nil {
		return
	}
	if s.insertMembershipStmt, err = db.Prepare(insertMembershipSQL); err != nil {
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

func (s *membershipStatements) insertMembership(localpart string, roomID string) (err error) {
	fmt.Printf("Inserting membership for user %s and room %s\n", localpart, roomID)
	_, err = s.insertMembershipStmt.Exec(localpart, roomID)
	fmt.Println(err)
	return
}

func (s *membershipStatements) deleteMembership(localpart string, roomID string) (err error) {
	_, err = s.deleteMembershipStmt.Exec(localpart, roomID)
	return
}
