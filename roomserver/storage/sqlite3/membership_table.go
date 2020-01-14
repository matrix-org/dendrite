// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type membershipState int64

const (
	membershipStateLeaveOrBan membershipState = 1
	membershipStateInvite     membershipState = 2
	membershipStateJoin       membershipState = 3
)

const membershipSchema = `
	CREATE TABLE IF NOT EXISTS roomserver_membership (
		room_nid INTEGER NOT NULL,
		target_nid INTEGER NOT NULL,
		sender_nid INTEGER NOT NULL DEFAULT 0,
		membership_nid INTEGER NOT NULL DEFAULT 1,
		event_nid INTEGER NOT NULL DEFAULT 0,
		UNIQUE (room_nid, target_nid)
	);
`

// Insert a row in to membership table so that it can be locked by the
// SELECT FOR UPDATE
const insertMembershipSQL = "" +
	"INSERT INTO roomserver_membership (room_nid, target_nid)" +
	" VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

const selectMembershipFromRoomAndTargetSQL = "" +
	"SELECT membership_nid, event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND target_nid = $2"

const selectMembershipsFromRoomAndMembershipSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND membership_nid = $2"

const selectMembershipsFromRoomSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1"

const selectMembershipForUpdateSQL = "" +
	"SELECT membership_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND target_nid = $2"

const updateMembershipSQL = "" +
	"UPDATE roomserver_membership SET sender_nid = $3, membership_nid = $4, event_nid = $5" +
	" WHERE room_nid = $1 AND target_nid = $2"

type membershipStatements struct {
	insertMembershipStmt                       *sql.Stmt
	selectMembershipForUpdateStmt              *sql.Stmt
	selectMembershipFromRoomAndTargetStmt      *sql.Stmt
	selectMembershipsFromRoomAndMembershipStmt *sql.Stmt
	selectMembershipsFromRoomStmt              *sql.Stmt
	updateMembershipStmt                       *sql.Stmt
}

func (s *membershipStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(membershipSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertMembershipStmt, insertMembershipSQL},
		{&s.selectMembershipForUpdateStmt, selectMembershipForUpdateSQL},
		{&s.selectMembershipFromRoomAndTargetStmt, selectMembershipFromRoomAndTargetSQL},
		{&s.selectMembershipsFromRoomAndMembershipStmt, selectMembershipsFromRoomAndMembershipSQL},
		{&s.selectMembershipsFromRoomStmt, selectMembershipsFromRoomSQL},
		{&s.updateMembershipStmt, updateMembershipSQL},
	}.prepare(db)
}

func (s *membershipStatements) insertMembership(
	ctx context.Context,
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) error {
	stmt := common.TxStmt(txn, s.insertMembershipStmt)
	_, err := stmt.ExecContext(ctx, roomNID, targetUserNID)
	if err != nil {
		fmt.Println("insertMembership stmt.ExecContent:", err)
	}
	return err
}

func (s *membershipStatements) selectMembershipForUpdate(
	ctx context.Context,
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (membership membershipState, err error) {
	stmt := common.TxStmt(txn, s.selectMembershipForUpdateStmt)
	err = stmt.QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership)
	if err != nil {
		fmt.Println("selectMembershipForUpdate common.TxStmt.Scan:", err)
	}
	return
}

func (s *membershipStatements) selectMembershipFromRoomAndTarget(
	ctx context.Context,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (eventNID types.EventNID, membership membershipState, err error) {
	err = s.selectMembershipFromRoomAndTargetStmt.QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership, &eventNID)
	if err != nil {
		fmt.Println("selectMembershipForUpdate s.selectMembershipFromRoomAndTargetStmt.QueryRowContext:", err)
	}
	return
}

func (s *membershipStatements) selectMembershipsFromRoom(
	ctx context.Context, roomNID types.RoomNID,
) (eventNIDs []types.EventNID, err error) {
	rows, err := s.selectMembershipsFromRoomStmt.QueryContext(ctx, roomNID)
	if err != nil {
		fmt.Println("selectMembershipsFromRoom s.selectMembershipsFromRoomStmt.QueryContext:", err)
		return
	}

	for rows.Next() {
		var eNID types.EventNID
		if err = rows.Scan(&eNID); err != nil {
			fmt.Println("selectMembershipsFromRoom rows.Scan:", err)
			return
		}
		eventNIDs = append(eventNIDs, eNID)
	}
	return
}
func (s *membershipStatements) selectMembershipsFromRoomAndMembership(
	ctx context.Context,
	roomNID types.RoomNID, membership membershipState,
) (eventNIDs []types.EventNID, err error) {
	stmt := s.selectMembershipsFromRoomAndMembershipStmt
	rows, err := stmt.QueryContext(ctx, roomNID, membership)
	if err != nil {
		fmt.Println("selectMembershipsFromRoomAndMembership stmt.QueryContext:", err)
		return
	}

	for rows.Next() {
		var eNID types.EventNID
		if err = rows.Scan(&eNID); err != nil {
			fmt.Println("selectMembershipsFromRoomAndMembership rows.Scan:", err)
			return
		}
		eventNIDs = append(eventNIDs, eNID)
	}
	return
}

func (s *membershipStatements) updateMembership(
	ctx context.Context,
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
	senderUserNID types.EventStateKeyNID, membership membershipState,
	eventNID types.EventNID,
) error {
	stmt := common.TxStmt(txn, s.updateMembershipStmt)
	_, err := stmt.ExecContext(
		ctx, roomNID, targetUserNID, senderUserNID, membership, eventNID,
	)
	if err != nil {
		fmt.Println("updateMembership common.TxStmt.ExecContent:", err)
	}
	return err
}
