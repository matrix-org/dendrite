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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const inviteSchema = `
CREATE TABLE IF NOT EXISTS roomserver_invites (
	-- The string ID of the invite event itself.
	-- We can't use a numeric event ID here because we don't always have
	-- enough information to store an invite in the event table.
	-- In particular we don't always have a chain of auth_events for invites
	-- received over federation.
	invite_event_id TEXT PRIMARY KEY,
	-- The numeric ID of the room the invite m.room.member event is in.
	room_nid BIGINT NOT NULL,
	-- The numeric ID for the state key of the invite m.room.member event.
	-- This tells us who the invite is for.
	-- This is used to query the active invites for a user.
	target_nid BIGINT NOT NULL,
	-- The numeric ID for the sender of the invite m.room.member event.
	-- This tells us who sent the invite.
	-- This is used to work out which matrix server we should talk to when
	-- we try to join the room.
	sender_nid BIGINT NOT NULL DEFAULT 0,
	-- This is used to track whether the invite is still active.
	-- This is set implicitly when processing new join and leave events and
	-- explicitly when rejecting events over federation.
	retired BOOLEAN NOT NULL DEFAULT FALSE,
	-- The invite event JSON.
	invite_event_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS roomserver_invites_active_idx ON roomserver_invites (target_nid, room_nid)
	WHERE NOT retired;
`
const insertInviteEventSQL = "" +
	"INSERT INTO roomserver_invites (invite_event_id, room_nid, target_nid," +
	" sender_nid, invite_event_json) VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT DO NOTHING"

const selectInviteActiveForUserInRoomSQL = "" +
	"SELECT invite_event_id, sender_nid, invite_event_json FROM roomserver_invites" +
	" WHERE target_nid = $1 AND room_nid = $2" +
	" AND NOT retired"

// Retire every active invite for a user in a room.
// Ideally we'd know which invite events were retired by a given update so we
// wouldn't need to remove every active invite.
// However the matrix protocol doesn't give us a way to reliably identify the
// invites that were retired, so we are forced to retire all of them.
const updateInviteRetiredSQL = "" +
	"UPDATE roomserver_invites SET retired = TRUE" +
	" WHERE room_nid = $1 AND target_nid = $2 AND NOT retired" +
	" RETURNING invite_event_id"

type inviteStatements struct {
	insertInviteEventStmt               *sql.Stmt
	selectInviteActiveForUserInRoomStmt *sql.Stmt
	updateInviteRetiredStmt             *sql.Stmt
}

func CreateInvitesTable(db *sql.DB) error {
	_, err := db.Exec(inviteSchema)
	return err
}

func PrepareInvitesTable(db *sql.DB) (tables.Invites, error) {
	s := &inviteStatements{}

	return s, sqlutil.StatementList{
		{&s.insertInviteEventStmt, insertInviteEventSQL},
		{&s.selectInviteActiveForUserInRoomStmt, selectInviteActiveForUserInRoomSQL},
		{&s.updateInviteRetiredStmt, updateInviteRetiredSQL},
	}.Prepare(db)
}

func (s *inviteStatements) InsertInviteEvent(
	ctx context.Context, txn *sql.Tx,
	inviteEventID string, roomNID types.RoomNID,
	targetUserNID, senderUserNID types.EventStateKeyNID,
	inviteEventJSON []byte,
) (bool, error) {
	result, err := sqlutil.TxStmt(txn, s.insertInviteEventStmt).ExecContext(
		ctx, inviteEventID, roomNID, targetUserNID, senderUserNID, inviteEventJSON,
	)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count != 0, nil
}

func (s *inviteStatements) UpdateInviteRetired(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) ([]string, error) {
	stmt := sqlutil.TxStmt(txn, s.updateInviteRetiredStmt)
	rows, err := stmt.QueryContext(ctx, roomNID, targetUserNID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "updateInviteRetired: rows.close() failed")

	var eventIDs []string
	var inviteEventID string
	for rows.Next() {
		if err = rows.Scan(&inviteEventID); err != nil {
			return nil, err
		}
		eventIDs = append(eventIDs, inviteEventID)
	}
	return eventIDs, rows.Err()
}

// SelectInviteActiveForUserInRoom returns a list of sender state key NIDs
func (s *inviteStatements) SelectInviteActiveForUserInRoom(
	ctx context.Context, txn *sql.Tx,
	targetUserNID types.EventStateKeyNID, roomNID types.RoomNID,
) ([]types.EventStateKeyNID, []string, []byte, error) {
	stmt := sqlutil.TxStmt(txn, s.selectInviteActiveForUserInRoomStmt)
	rows, err := stmt.QueryContext(
		ctx, targetUserNID, roomNID,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectInviteActiveForUserInRoom: rows.close() failed")
	var result []types.EventStateKeyNID
	var eventIDs []string
	var inviteEventID string
	var senderUserNID int64
	var eventJSON []byte
	for rows.Next() {
		if err := rows.Scan(&inviteEventID, &senderUserNID, &eventJSON); err != nil {
			return nil, nil, nil, err
		}
		result = append(result, types.EventStateKeyNID(senderUserNID))
		eventIDs = append(eventIDs, inviteEventID)
	}
	return result, eventIDs, eventJSON, rows.Err()
}
