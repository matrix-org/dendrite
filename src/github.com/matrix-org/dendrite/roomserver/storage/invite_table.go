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

const inviteSchema = `
CREATE TABLE invites (
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
	-- This is set implicitly when processing KIND_NEW events and explicitly
	-- when rejecting events over federation.
	retired BOOLEAN NOT NULL DEFAULT FALSE,
	-- Whether the invite has been sent to the output stream.
	-- We maintain a separate output stream of invite events since they don't
	-- always occur within a room we have state for.
	sent_invite_to_output BOOLEAN NOT NULL DEFAULT FALSE,
	-- Whether the retirement has been sent to the output stream.
	sent_retired_to_output BOOLEAN NOT NULL DEFAULT FALSE,
	-- The invite event JSON.
	invite_event_json TEXT NOT NULL
);

CREATE INDEX invites_active_idx ON invites (target_state_key_nid, room_nid)
	WHERE NOT retired;

CREATE INDEX invites_unsent_retired_idx ON invites (target_state_key_nid, room_nid)
	WHERE retired AND NOT sent_retired_to_output;
`

const insertInviteEventSQL = "" +
	"INSERT INTO invites (invite_event_id, room_nid, target_state_key_nid," +
	" sender_state_key_nid, invite_event_json) VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT DO NOTHING"

const selectInviteSQL = "" +
	"SELECT retired, sent_invite_to_output FROM invites" +
	" WHERE invite_event_id = $1"

const updateInviteSentInviteToOutputSQL = "" +
	"UPDATE invites SET sent_invite_to_output = TRUE" +
	" WHERE invite_event_id = $1"

const selectInviteActiveForUserInRoomSQL = "" +
	"SELECT invite_event_id, sender_state_key_nid FROM invites" +
	" WHERE target_state_key_id = $1 AND room_nid = $2" +
	" AND NOT retired"

// Retire every active invite.
// Ideally we'd know which invite events were retired by a given update so we
// wouldn't need to remove every active invite.
// However the matrix protocol doesn't give us a way to reliably identify the
// invites that were retired, so we are forced to retire all of them.
const updateInviteRetiredSQL = "" +
	"UPDATE invites SET retired_by_event_nid = TRUE" +
	" WHERE room_nid = $1 AND target_state_key_nid = $2 AND NOT retired"

const selectInviteUnsentRetiredSQL = "" +
	"SELECT invite_event_id FROM invites" +
	" WHERE target_state_key_id = $1 AND room_nid = $2" +
	" AND retired AND NOT sent_retired_to_output"

const updateInviteSentRetiredToOutputSQL = "" +
	"UPDATE invites SET sent_retired_to_output = TRUE" +
	" WHERE invite_event_id = $1"

type inviteStatements struct {
	insertInviteEventStmt               *sql.Stmt
	selectInviteStmt                    *sql.Stmt
	selectInviteActiveForUserInRoomStmt *sql.Stmt
	updateInviteRetiredStmt             *sql.Stmt
	selectInviteUnsentRetiredStmt       *sql.Stmt
	updateInviteSentInviteToOutputStmt  *sql.Stmt
	updateInviteSentRetiredToOutputStmt *sql.Stmt
}

func (s *inviteStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(inviteSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertInviteEventStmt, insertInviteEventSQL},
		{&s.selectInviteStmt, selectInviteSQL},
		{&s.updateInviteSentInviteToOutputStmt, updateInviteSentInviteToOutputSQL},
		{&s.selectInviteActiveForUserInRoomStmt, selectInviteActiveForUserInRoomSQL},
		{&s.updateInviteRetiredStmt, updateInviteRetiredSQL},

		{&s.updateInviteSentRetiredToOutputStmt, updateInviteSentRetiredToOutputSQL},
	}.prepare(db)
}

func (s *inviteStatements) insertInviteEvent(
	txn *sql.Tx, inviteEventNID types.EventNID, roomNID types.RoomNID,
	targetNID, senderNID types.EventStateKeyNID,
	inviteEventJSON []byte,
) error {
	_, err := txn.Stmt(s.insertInviteEventStmt).Exec(
		inviteEventNID, roomNID, targetNID, senderNID, inviteEventJSON,
	)
	return err
}

func (s *inviteStatements) updateInviteRetired(
	txn *sql.Tx, roomNID types.RoomNID, targetNID types.EventStateKeyNID,
) error {
	_, err := txn.Stmt(s.updateInviteRetiredStmt).Exec(roomNID, targetNID)
	return err
}

func (s *inviteStatements) selectInvite(
	txn *sql.Tx, inviteEventNID types.EventNID,
) (RetiredByNID types.EventNID, sentInviteToOutput, sentRetiredToOutput bool, err error) {
	err = txn.Stmt(s.selectInviteStmt).QueryRow(inviteEventNID).Scan(
		&RetiredByNID, &sentInviteToOutput, &sentRetiredToOutput,
	)
	return
}

// selectInviteActiveForUserInRoom returns a list of sender state key NIDs
func (s *inviteStatements) selectInviteActiveForUserInRoom(
	targetNID types.EventStateKeyNID, roomNID types.RoomNID,
) ([]types.EventStateKeyNID, error) {
	rows, err := s.selectInviteActiveForUserInRoomStmt.Query(
		targetNID, roomNID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []types.EventStateKeyNID
	for rows.Next() {
		var senderNID int64
		if err := rows.Scan(&senderNID); err != nil {
			return nil, err
		}
		result = append(result, types.EventStateKeyNID(senderNID))
	}
	return result, nil
}

func (s *inviteStatements) updateInviteSentInviteToOutput(
	inviteEventNID types.EventNID,
) error {
	_, err := s.updateInviteSentInviteToOutputStmt.Exec(inviteEventNID)
	return err
}

func (s *inviteStatements) updateInviteSentRetiredToOutput(
	inviteEventNID types.EventNID,
) error {
	_, err := s.updateInviteSentRetiredToOutputStmt.Exec(inviteEventNID)
	return err
}
