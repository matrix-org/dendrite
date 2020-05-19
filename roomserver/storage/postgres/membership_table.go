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

package postgres

import (
	"context"
	"database/sql"

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
	-- The numeric ID of the membership event.
	-- It refers to the join membership event if the membership_nid is join (3),
	-- and to the leave/ban membership event if the membership_nid is leave or
	-- ban (1).
	-- If the membership_nid is invite (2) and the user has been in the room
	-- before, it will refer to the previous leave/ban membership event, and will
	-- be equals to 0 (its default) if the user never joined the room before.
	-- This NID is updated if the join event gets updated (e.g. profile update),
	-- or if the user leaves/joins the room.
	event_nid BIGINT NOT NULL DEFAULT 0,
	-- Local target is true if the target_nid refers to a local user rather than
	-- a federated one. This is an optimisation for resetting state on federated
	-- room joins.
	local_target BOOLEAN NOT NULL DEFAULT false,
	UNIQUE (room_nid, target_nid)
);
`

// Insert a row in to membership table so that it can be locked by the
// SELECT FOR UPDATE
const insertMembershipSQL = "" +
	"INSERT INTO roomserver_membership (room_nid, target_nid, local_target)" +
	" VALUES ($1, $2, $3)" +
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
	" WHERE room_nid = $1 AND target_nid = $2 FOR UPDATE"

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
	localTarget bool,
) error {
	stmt := common.TxStmt(txn, s.insertMembershipStmt)
	_, err := stmt.ExecContext(ctx, roomNID, targetUserNID, localTarget)
	return err
}

func (s *membershipStatements) selectMembershipForUpdate(
	ctx context.Context,
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (membership membershipState, err error) {
	err = common.TxStmt(txn, s.selectMembershipForUpdateStmt).QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership)
	return
}

func (s *membershipStatements) selectMembershipFromRoomAndTarget(
	ctx context.Context,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (eventNID types.EventNID, membership membershipState, err error) {
	err = s.selectMembershipFromRoomAndTargetStmt.QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership, &eventNID)
	return
}

func (s *membershipStatements) selectMembershipsFromRoom(
	ctx context.Context, roomNID types.RoomNID,
) (eventNIDs []types.EventNID, err error) {
	rows, err := s.selectMembershipsFromRoomStmt.QueryContext(ctx, roomNID)
	if err != nil {
		return
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectMembershipsFromRoom: rows.close() failed")

	for rows.Next() {
		var eNID types.EventNID
		if err = rows.Scan(&eNID); err != nil {
			return
		}
		eventNIDs = append(eventNIDs, eNID)
	}
	return eventNIDs, rows.Err()
}

func (s *membershipStatements) selectMembershipsFromRoomAndMembership(
	ctx context.Context,
	roomNID types.RoomNID, membership membershipState,
) (eventNIDs []types.EventNID, err error) {
	stmt := s.selectMembershipsFromRoomAndMembershipStmt
	rows, err := stmt.QueryContext(ctx, roomNID, membership)
	if err != nil {
		return
	}
	defer common.CloseAndLogIfError(ctx, rows, "selectMembershipsFromRoomAndMembership: rows.close() failed")

	for rows.Next() {
		var eNID types.EventNID
		if err = rows.Scan(&eNID); err != nil {
			return
		}
		eventNIDs = append(eventNIDs, eNID)
	}
	return eventNIDs, rows.Err()
}

func (s *membershipStatements) updateMembership(
	ctx context.Context,
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
	senderUserNID types.EventStateKeyNID, membership membershipState,
	eventNID types.EventNID,
) error {
	_, err := common.TxStmt(txn, s.updateMembershipStmt).ExecContext(
		ctx, roomNID, targetUserNID, senderUserNID, membership, eventNID,
	)
	return err
}
