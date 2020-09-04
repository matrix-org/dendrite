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
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const membershipSchema = `
	CREATE TABLE IF NOT EXISTS roomserver_membership (
		room_nid INTEGER NOT NULL,
		target_nid INTEGER NOT NULL,
		sender_nid INTEGER NOT NULL DEFAULT 0,
		membership_nid INTEGER NOT NULL DEFAULT 1,
		event_nid INTEGER NOT NULL DEFAULT 0,
		target_local BOOLEAN NOT NULL DEFAULT false,
		UNIQUE (room_nid, target_nid)
	);
`

var selectJoinedUsersSetForRoomsSQL = "" +
	"SELECT target_nid, COUNT(room_nid) FROM roomserver_membership WHERE room_nid IN ($1) AND" +
	" membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " GROUP BY target_nid"

// Insert a row in to membership table so that it can be locked by the
// SELECT FOR UPDATE
const insertMembershipSQL = "" +
	"INSERT INTO roomserver_membership (room_nid, target_nid, target_local)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT DO NOTHING"

const selectMembershipFromRoomAndTargetSQL = "" +
	"SELECT membership_nid, event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND target_nid = $2"

const selectMembershipsFromRoomAndMembershipSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND membership_nid = $2"

const selectLocalMembershipsFromRoomAndMembershipSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND membership_nid = $2" +
	" AND target_local = true"

const selectMembershipsFromRoomSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1"

const selectLocalMembershipsFromRoomSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1" +
	" AND target_local = true"

const selectMembershipForUpdateSQL = "" +
	"SELECT membership_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND target_nid = $2"

const updateMembershipSQL = "" +
	"UPDATE roomserver_membership SET sender_nid = $1, membership_nid = $2, event_nid = $3" +
	" WHERE room_nid = $4 AND target_nid = $5"

const selectRoomsWithMembershipSQL = "" +
	"SELECT room_nid FROM roomserver_membership WHERE membership_nid = $1 AND target_nid = $2"

// selectKnownUsersSQL uses a sub-select statement here to find rooms that the user is
// joined to. Since this information is used to populate the user directory, we will
// only return users that the user would ordinarily be able to see anyway.
var selectKnownUsersSQL = "" +
	"SELECT DISTINCT event_state_key FROM roomserver_membership INNER JOIN roomserver_event_state_keys ON " +
	"roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
	" WHERE room_nid IN (" +
	"  SELECT DISTINCT room_nid FROM roomserver_membership WHERE target_nid=$1 AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) +
	") AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " AND event_state_key LIKE $2 LIMIT $3"

type membershipStatements struct {
	db                                              *sql.DB
	insertMembershipStmt                            *sql.Stmt
	selectMembershipForUpdateStmt                   *sql.Stmt
	selectMembershipFromRoomAndTargetStmt           *sql.Stmt
	selectMembershipsFromRoomAndMembershipStmt      *sql.Stmt
	selectLocalMembershipsFromRoomAndMembershipStmt *sql.Stmt
	selectMembershipsFromRoomStmt                   *sql.Stmt
	selectLocalMembershipsFromRoomStmt              *sql.Stmt
	selectRoomsWithMembershipStmt                   *sql.Stmt
	updateMembershipStmt                            *sql.Stmt
	selectKnownUsersStmt                            *sql.Stmt
}

func NewSqliteMembershipTable(db *sql.DB) (tables.Membership, error) {
	s := &membershipStatements{
		db: db,
	}
	_, err := db.Exec(membershipSchema)
	if err != nil {
		return nil, err
	}

	return s, shared.StatementList{
		{&s.insertMembershipStmt, insertMembershipSQL},
		{&s.selectMembershipForUpdateStmt, selectMembershipForUpdateSQL},
		{&s.selectMembershipFromRoomAndTargetStmt, selectMembershipFromRoomAndTargetSQL},
		{&s.selectMembershipsFromRoomAndMembershipStmt, selectMembershipsFromRoomAndMembershipSQL},
		{&s.selectLocalMembershipsFromRoomAndMembershipStmt, selectLocalMembershipsFromRoomAndMembershipSQL},
		{&s.selectMembershipsFromRoomStmt, selectMembershipsFromRoomSQL},
		{&s.selectLocalMembershipsFromRoomStmt, selectLocalMembershipsFromRoomSQL},
		{&s.updateMembershipStmt, updateMembershipSQL},
		{&s.selectRoomsWithMembershipStmt, selectRoomsWithMembershipSQL},
		{&s.selectKnownUsersStmt, selectKnownUsersSQL},
	}.Prepare(db)
}

func (s *membershipStatements) InsertMembership(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
	localTarget bool,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertMembershipStmt)
	_, err := stmt.ExecContext(ctx, roomNID, targetUserNID, localTarget)
	return err
}

func (s *membershipStatements) SelectMembershipForUpdate(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (membership tables.MembershipState, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectMembershipForUpdateStmt)
	err = stmt.QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership)
	return
}

func (s *membershipStatements) SelectMembershipFromRoomAndTarget(
	ctx context.Context,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (eventNID types.EventNID, membership tables.MembershipState, err error) {
	err = s.selectMembershipFromRoomAndTargetStmt.QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership, &eventNID)
	return
}

func (s *membershipStatements) SelectMembershipsFromRoom(
	ctx context.Context,
	roomNID types.RoomNID, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	var selectStmt *sql.Stmt
	if localOnly {
		selectStmt = s.selectLocalMembershipsFromRoomStmt
	} else {
		selectStmt = s.selectMembershipsFromRoomStmt
	}
	rows, err := selectStmt.QueryContext(ctx, roomNID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectMembershipsFromRoom: rows.close() failed")

	for rows.Next() {
		var eNID types.EventNID
		if err = rows.Scan(&eNID); err != nil {
			return
		}
		eventNIDs = append(eventNIDs, eNID)
	}
	return
}

func (s *membershipStatements) SelectMembershipsFromRoomAndMembership(
	ctx context.Context,
	roomNID types.RoomNID, membership tables.MembershipState, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	var stmt *sql.Stmt
	if localOnly {
		stmt = s.selectLocalMembershipsFromRoomAndMembershipStmt
	} else {
		stmt = s.selectMembershipsFromRoomAndMembershipStmt
	}
	rows, err := stmt.QueryContext(ctx, roomNID, membership)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectMembershipsFromRoomAndMembership: rows.close() failed")

	for rows.Next() {
		var eNID types.EventNID
		if err = rows.Scan(&eNID); err != nil {
			return
		}
		eventNIDs = append(eventNIDs, eNID)
	}
	return
}

func (s *membershipStatements) UpdateMembership(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
	senderUserNID types.EventStateKeyNID, membership tables.MembershipState,
	eventNID types.EventNID,
) error {
	stmt := sqlutil.TxStmt(txn, s.updateMembershipStmt)
	_, err := stmt.ExecContext(
		ctx, senderUserNID, membership, eventNID, roomNID, targetUserNID,
	)
	return err
}

func (s *membershipStatements) SelectRoomsWithMembership(
	ctx context.Context, userID types.EventStateKeyNID, membershipState tables.MembershipState,
) ([]types.RoomNID, error) {
	rows, err := s.selectRoomsWithMembershipStmt.QueryContext(ctx, membershipState, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRoomsWithMembership: rows.close() failed")
	var roomNIDs []types.RoomNID
	for rows.Next() {
		var roomNID types.RoomNID
		if err := rows.Scan(&roomNID); err != nil {
			return nil, err
		}
		roomNIDs = append(roomNIDs, roomNID)
	}
	return roomNIDs, nil
}

func (s *membershipStatements) SelectJoinedUsersSetForRooms(ctx context.Context, roomNIDs []types.RoomNID) (map[types.EventStateKeyNID]int, error) {
	iRoomNIDs := make([]interface{}, len(roomNIDs))
	for i, v := range roomNIDs {
		iRoomNIDs[i] = v
	}
	query := strings.Replace(selectJoinedUsersSetForRoomsSQL, "($1)", sqlutil.QueryVariadic(len(iRoomNIDs)), 1)
	rows, err := s.db.QueryContext(ctx, query, iRoomNIDs...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJoinedUsersSetForRooms: rows.close() failed")
	result := make(map[types.EventStateKeyNID]int)
	for rows.Next() {
		var userID types.EventStateKeyNID
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, err
		}
		result[userID] = count
	}
	return result, rows.Err()
}

func (s *membershipStatements) SelectKnownUsers(ctx context.Context, userID types.EventStateKeyNID, searchString string, limit int) ([]string, error) {
	rows, err := s.selectKnownUsersStmt.QueryContext(ctx, userID, fmt.Sprintf("%%%s%%", searchString), limit)
	if err != nil {
		return nil, err
	}
	result := []string{}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectKnownUsers: rows.close() failed")
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		result = append(result, userID)
	}
	return result, rows.Err()
}
