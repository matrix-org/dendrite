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
	"github.com/matrix-org/dendrite/roomserver/types"
)

type membershipState int64

const (
	membershipStateLeaveOrBan membershipState = 1
	membershipStateInvite     membershipState = 2
	membershipStateJoin       membershipState = 3
)

const membershipSchema = `
-- The membership table is used to coordinate updates between the invite table
-- and the room state tables.
-- This table is updated in one of 3 ways:
--   1) The membership of a user changes within the current state of the room.
--   2) An invite is received outside of a room over federation.
--   3) An invite is rejected outside of a room over federation.
CREATE TABLE IF NOT EXISTS roomserver_membership (
	room_nid BIGINT NOT NULL,
	-- Numeric state key ID for the user ID this state is for.
	target_nid BIGINT NOT NULL,
	-- Numeric state key ID for the user ID who changed the state.
	-- This may be 0 since it is not always possible to identify the user that
	-- changed the state.
	sender_nid BIGINT NOT NULL DEFAULT 0,
	-- The state the user is in within this room.
	-- Default value is "membershipStateLeaveOrBan"
	membership_nid BIGINT NOT NULL DEFAULT 1,
	UNIQUE (room_nid, target_nid)
);
`

// Insert a row in to membership table so that it can be locked by the
// SELECT FOR UPDATE
const insertMembershipSQL = "" +
	"INSERT INTO roomserver_membership (room_nid, target_nid)" +
	" VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

const selectMembershipForUpdateSQL = "" +
	"SELECT membership_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND target_nid = $2 FOR UPDATE"

const countJoinedMembersInRoomsSQL = "" +
	"SELECT room_nid, COUNT(*) FROM roomserver_membership" +
	" WHERE room_nid = ANY($1) AND membership_nid = $2" +
	" GROUP BY room_nid"

const updateMembershipSQL = "" +
	"UPDATE roomserver_membership SET sender_nid = $3, membership_nid = $4" +
	" WHERE room_nid = $1 AND target_nid = $2"

type membershipStatements struct {
	insertMembershipStmt          *sql.Stmt
	selectMembershipForUpdateStmt *sql.Stmt
	countJoinedMembersInRoomsStmt *sql.Stmt
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
		{&s.countJoinedMembersInRoomsStmt, countJoinedMembersInRoomsSQL},
		{&s.updateMembershipStmt, updateMembershipSQL},
	}.prepare(db)
}

func (s *membershipStatements) insertMembership(
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) error {
	_, err := txn.Stmt(s.insertMembershipStmt).Exec(roomNID, targetUserNID)
	return err
}

func (s *membershipStatements) selectMembershipForUpdate(
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (membership membershipState, err error) {
	err = txn.Stmt(s.selectMembershipForUpdateStmt).QueryRow(
		roomNID, targetUserNID,
	).Scan(&membership)
	return
}

func (s *membershipStatements) countJoinedMembersInRooms(roomNIDs []types.RoomNID) (map[types.RoomNID]int64, error) {
	nIDs := []int64{}
	for _, roomNID := range roomNIDs {
		nIDs = append(nIDs, int64(roomNID))
	}

	rows, err := s.countJoinedMembersInRoomsStmt.Query(pq.Int64Array(nIDs), membershipStateJoin)
	if err != nil {
		return nil, err
	}

	members := make(map[types.RoomNID]int64)
	for rows.Next() {
		var roomNID types.RoomNID
		var count int64

		if err = rows.Scan(&roomNID, &count); err != nil {
			return members, err
		}

		members[roomNID] = count
	}

	return members, err
}

func (s *membershipStatements) updateMembership(
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
	senderUserNID types.EventStateKeyNID, membership membershipState,
) error {
	_, err := txn.Stmt(s.updateMembershipStmt).Exec(
		roomNID, targetUserNID, senderUserNID, membership,
	)
	return err
}
