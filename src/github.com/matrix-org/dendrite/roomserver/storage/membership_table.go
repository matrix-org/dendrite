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

	"github.com/matrix-org/dendrite/roomserver/types"
)

type membershipState int64

const (
	membershipStateLeaveOrBan membershipState = 0
	membershipStateInvite     membershipState = 1
	membershipStateJoin       membershipState = 2
)

const membershipSchema = `
-- The membership table is used to coordinate updates between the invite table
-- and the room state tables.
-- This table is updated in one of 3 ways:
--   1) The membership of a user changes within the current state of the room.
--   2) An invite is received outside of a room over federation. 
--   3) An invite is rejected outside of a room over federation.
CREATE TABLE membership IF NOT EXISTS (
	room_nid BIGINT NOT NULL,
	-- Numeric state key ID for the user ID this state is for.
	target_nid BIGINT NOT NULL,
	-- Numeric state key ID for the user ID who changed the state.
	sender_nid BIGINT NOT NULL DEFAULT 0,
	-- The state the user is in within this room.
	membership_nid BIGINT NOT NULL DEFAULT 0,
	UNIQUE (room_nid, target_nid)
);
`

const insertMembershipSQL = "" +
	"INSERT INTO membership (room_nid, target_nid)" +
	" VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

const selectMembershipForUpdateSQL = "" +
	"SELECT membership_nid FROM membership" +
	" WHERE room_nid = $1, target_nid = $2 FOR UPDATE"

const updateMembershipSQL = "" +
	"UPDATE membership SET membership_nid = $3, sender_nid = $4" +
	" WHERE room_nid = $1, target_nid = $2"

type membershipStatements struct {
	insertMembershipStmt          *sql.Stmt
	selectMembershipForUpdateStmt *sql.Stmt
	updateMembershipStmt          *sql.Stmt
}

func (s *membershipStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(membershipSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertMembershipStmt, insertMembershipSQL},
		{&s.selectMembershipForUpdateStmt, selectMembershipForUpdateSQL},
		{&s.updateMembershipStmt, updateMembershipSQL},
	}.prepare(db)
}

func (s *membershipStatements) insertMembership(
	txn *sql.Tx, roomNID types.RoomNID, targetNID types.EventStateKeyNID,
) error {
	_, err := txn.Stmt(s.insertMembershipStmt).Exec(roomNID, targetNID)
	return err
}

func (s *membershipStatements) selectMembershipForUpdate(
	txn *sql.Tx, roomNID types.RoomNID, targetNID types.EventStateKeyNID,
) (membership membershipState, err error) {
	err = txn.Stmt(s.selectMembershipForUpdateStmt).QueryRow(
		roomNID, targetNID,
	).Scan(&membership)
	return
}

func (s *membershipStatements) updateMembershipSQL(
	txn *sql.Tx, roomNID types.RoomNID, targetNID types.EventStateKeyNID,
	senderNID types.EventStateKeyNID, membership membershipState,
) error {
	_, err := txn.Stmt(s.updateMembershipStmt).Exec(
		roomNID, targetNID, senderNID, membership,
	)
	return err
}
