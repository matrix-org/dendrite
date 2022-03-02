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
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
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
	target_local BOOLEAN NOT NULL DEFAULT false,
	forgotten BOOLEAN NOT NULL DEFAULT FALSE,
	UNIQUE (room_nid, target_nid)
);
`

var selectJoinedUsersSetForRoomsSQL = "" +
	"SELECT target_nid, COUNT(room_nid) FROM roomserver_membership" +
	" WHERE room_nid = ANY($1) AND target_nid = ANY($2) AND" +
	" membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " and forgotten = false" +
	" GROUP BY target_nid"

// Insert a row in to membership table so that it can be locked by the
// SELECT FOR UPDATE
const insertMembershipSQL = "" +
	"INSERT INTO roomserver_membership (room_nid, target_nid, target_local)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT DO NOTHING"

const selectMembershipFromRoomAndTargetSQL = "" +
	"SELECT membership_nid, event_nid, forgotten FROM roomserver_membership" +
	" WHERE room_nid = $1 AND target_nid = $2"

const selectMembershipsFromRoomAndMembershipSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND membership_nid = $2 and forgotten = false"

const selectLocalMembershipsFromRoomAndMembershipSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND membership_nid = $2" +
	" AND target_local = true and forgotten = false"

const selectMembershipsFromRoomSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 and forgotten = false"

const selectLocalMembershipsFromRoomSQL = "" +
	"SELECT event_nid FROM roomserver_membership" +
	" WHERE room_nid = $1" +
	" AND target_local = true and forgotten = false"

const selectMembershipForUpdateSQL = "" +
	"SELECT membership_nid FROM roomserver_membership" +
	" WHERE room_nid = $1 AND target_nid = $2 FOR UPDATE"

const updateMembershipSQL = "" +
	"UPDATE roomserver_membership SET sender_nid = $3, membership_nid = $4, event_nid = $5, forgotten = $6" +
	" WHERE room_nid = $1 AND target_nid = $2"

const updateMembershipForgetRoom = "" +
	"UPDATE roomserver_membership SET forgotten = $3" +
	" WHERE room_nid = $1 AND target_nid = $2"

const selectRoomsWithMembershipSQL = "" +
	"SELECT room_nid FROM roomserver_membership WHERE membership_nid = $1 AND target_nid = $2 and forgotten = false"

// selectKnownUsersSQL uses a sub-select statement here to find rooms that the user is
// joined to. Since this information is used to populate the user directory, we will
// only return users that the user would ordinarily be able to see anyway.
var selectKnownUsersSQL = "" +
	"SELECT DISTINCT event_state_key FROM roomserver_membership INNER JOIN roomserver_event_state_keys ON " +
	"roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
	" WHERE room_nid = ANY(" +
	"  SELECT DISTINCT room_nid FROM roomserver_membership WHERE target_nid=$1 AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) +
	") AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " AND event_state_key LIKE $2 LIMIT $3"

// selectLocalServerInRoomSQL is an optimised case for checking if we, the local server,
// are in the room by using the target_local column of the membership table. Normally when
// we want to know if a server is in a room, we have to unmarshal the entire room state which
// is expensive. The presence of a single row from this query suggests we're still in the
// room, no rows returned suggests we aren't.
const selectLocalServerInRoomSQL = "" +
	"SELECT room_nid FROM roomserver_membership WHERE target_local = true AND membership_nid = $1 AND room_nid = $2 LIMIT 1"

// selectServerMembersInRoomSQL is an optimised case for checking for server members in a room.
// The JOIN is significantly leaner than the previous case of looking up event NIDs and reading the
// membership events from the database, as the JOIN query amounts to little more than two index
// scans which are very fast. The presence of a single row from this query suggests the server is
// in the room, no rows returned suggests they aren't.
const selectServerInRoomSQL = "" +
	"SELECT room_nid FROM roomserver_membership" +
	" JOIN roomserver_event_state_keys ON roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
	" WHERE membership_nid = $1 AND room_nid = $2 AND event_state_key LIKE '%:' || $3 LIMIT 1"

type membershipStatements struct {
	insertMembershipStmt                            *sql.Stmt
	selectMembershipForUpdateStmt                   *sql.Stmt
	selectMembershipFromRoomAndTargetStmt           *sql.Stmt
	selectMembershipsFromRoomAndMembershipStmt      *sql.Stmt
	selectLocalMembershipsFromRoomAndMembershipStmt *sql.Stmt
	selectMembershipsFromRoomStmt                   *sql.Stmt
	selectLocalMembershipsFromRoomStmt              *sql.Stmt
	updateMembershipStmt                            *sql.Stmt
	selectRoomsWithMembershipStmt                   *sql.Stmt
	selectJoinedUsersSetForRoomsStmt                *sql.Stmt
	selectKnownUsersStmt                            *sql.Stmt
	updateMembershipForgetRoomStmt                  *sql.Stmt
	selectLocalServerInRoomStmt                     *sql.Stmt
	selectServerInRoomStmt                          *sql.Stmt
}

func createMembershipTable(db *sql.DB) error {
	_, err := db.Exec(membershipSchema)
	return err
}

func prepareMembershipTable(db *sql.DB) (tables.Membership, error) {
	s := &membershipStatements{}

	return s, sqlutil.StatementList{
		{&s.insertMembershipStmt, insertMembershipSQL},
		{&s.selectMembershipForUpdateStmt, selectMembershipForUpdateSQL},
		{&s.selectMembershipFromRoomAndTargetStmt, selectMembershipFromRoomAndTargetSQL},
		{&s.selectMembershipsFromRoomAndMembershipStmt, selectMembershipsFromRoomAndMembershipSQL},
		{&s.selectLocalMembershipsFromRoomAndMembershipStmt, selectLocalMembershipsFromRoomAndMembershipSQL},
		{&s.selectMembershipsFromRoomStmt, selectMembershipsFromRoomSQL},
		{&s.selectLocalMembershipsFromRoomStmt, selectLocalMembershipsFromRoomSQL},
		{&s.updateMembershipStmt, updateMembershipSQL},
		{&s.selectRoomsWithMembershipStmt, selectRoomsWithMembershipSQL},
		{&s.selectJoinedUsersSetForRoomsStmt, selectJoinedUsersSetForRoomsSQL},
		{&s.selectKnownUsersStmt, selectKnownUsersSQL},
		{&s.updateMembershipForgetRoomStmt, updateMembershipForgetRoom},
		{&s.selectLocalServerInRoomStmt, selectLocalServerInRoomSQL},
		{&s.selectServerInRoomStmt, selectServerInRoomSQL},
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
	err = sqlutil.TxStmt(txn, s.selectMembershipForUpdateStmt).QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership)
	return
}

func (s *membershipStatements) SelectMembershipFromRoomAndTarget(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (eventNID types.EventNID, membership tables.MembershipState, forgotten bool, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectMembershipFromRoomAndTargetStmt)
	err = stmt.QueryRowContext(
		ctx, roomNID, targetUserNID,
	).Scan(&membership, &eventNID, &forgotten)
	return
}

func (s *membershipStatements) SelectMembershipsFromRoom(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	var stmt *sql.Stmt
	if localOnly {
		stmt = s.selectLocalMembershipsFromRoomStmt
	} else {
		stmt = s.selectMembershipsFromRoomStmt
	}
	stmt = sqlutil.TxStmt(txn, stmt)
	rows, err := stmt.QueryContext(ctx, roomNID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectMembershipsFromRoom: rows.close() failed")

	for rows.Next() {
		var eNID types.EventNID
		if err = rows.Scan(&eNID); err != nil {
			return
		}
		eventNIDs = append(eventNIDs, eNID)
	}
	return eventNIDs, rows.Err()
}

func (s *membershipStatements) SelectMembershipsFromRoomAndMembership(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, membership tables.MembershipState, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	var rows *sql.Rows
	var stmt *sql.Stmt
	if localOnly {
		stmt = s.selectLocalMembershipsFromRoomAndMembershipStmt
	} else {
		stmt = s.selectMembershipsFromRoomAndMembershipStmt
	}
	stmt = sqlutil.TxStmt(txn, stmt)
	rows, err = stmt.QueryContext(ctx, roomNID, membership)
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
	return eventNIDs, rows.Err()
}

func (s *membershipStatements) UpdateMembership(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID, senderUserNID types.EventStateKeyNID, membership tables.MembershipState,
	eventNID types.EventNID, forgotten bool,
) error {
	_, err := sqlutil.TxStmt(txn, s.updateMembershipStmt).ExecContext(
		ctx, roomNID, targetUserNID, senderUserNID, membership, eventNID, forgotten,
	)
	return err
}

func (s *membershipStatements) SelectRoomsWithMembership(
	ctx context.Context, txn *sql.Tx,
	userID types.EventStateKeyNID, membershipState tables.MembershipState,
) ([]types.RoomNID, error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomsWithMembershipStmt)
	rows, err := stmt.QueryContext(ctx, membershipState, userID)
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

func (s *membershipStatements) SelectJoinedUsersSetForRooms(
	ctx context.Context, txn *sql.Tx,
	roomNIDs []types.RoomNID,
	userNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]int, error) {
	stmt := sqlutil.TxStmt(txn, s.selectJoinedUsersSetForRoomsStmt)
	rows, err := stmt.QueryContext(ctx, pq.Array(roomNIDs), pq.Array(userNIDs))
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

func (s *membershipStatements) SelectKnownUsers(
	ctx context.Context, txn *sql.Tx,
	userID types.EventStateKeyNID, searchString string, limit int,
) ([]string, error) {
	stmt := sqlutil.TxStmt(txn, s.selectKnownUsersStmt)
	rows, err := stmt.QueryContext(ctx, userID, fmt.Sprintf("%%%s%%", searchString), limit)
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

func (s *membershipStatements) UpdateForgetMembership(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID, forget bool,
) error {
	_, err := sqlutil.TxStmt(txn, s.updateMembershipForgetRoomStmt).ExecContext(
		ctx, roomNID, targetUserNID, forget,
	)
	return err
}

func (s *membershipStatements) SelectLocalServerInRoom(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID,
) (bool, error) {
	var nid types.RoomNID
	stmt := sqlutil.TxStmt(txn, s.selectLocalServerInRoomStmt)
	err := stmt.QueryRowContext(ctx, tables.MembershipStateJoin, roomNID).Scan(&nid)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	found := nid > 0
	return found, nil
}

func (s *membershipStatements) SelectServerInRoom(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, serverName gomatrixserverlib.ServerName,
) (bool, error) {
	var nid types.RoomNID
	stmt := sqlutil.TxStmt(txn, s.selectServerInRoomStmt)
	err := stmt.QueryRowContext(ctx, tables.MembershipStateJoin, roomNID, serverName).Scan(&nid)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return roomNID == nid, nil
}
