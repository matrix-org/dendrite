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
	-- The numeric ID of the invite event itself.
	invite_event_nid BIGINT NOT NULL CONSTRAINT invite_event_nid_unique UNIQUE,
	-- The numeric ID of the room the invite m.room.member event is in.
	room_nid BIGINT NOT NULL,
	-- The numeric ID for the state key of the invite m.room.member event.
	-- This tells us who the invite is for.
	-- This is used to query the active invites for a user.
	target_state_key_nid BIGINT NOT NULL,
	-- The numeric ID for the sender of the invite m.room.member event.
	-- This tells us who sent the invite.
	-- This is used to work out which matrix server we should talk to when
	-- we try to join the room.
	sender_state_key_nid BIGINT NOT NULL DEFAULT 0,
	-- The numeric ID of the join or leave event that replaced the invite event.
	-- This is used to track whether the invite is still active.
	-- This is set implicitly when processing a KIND_NEW events and explici
	-- is set 
	replaced_by_event_nid BIGINT NOT NULL DEFAULT 0,
	-- Whether the invite has been sent to the output stream.
	-- We maintain a separate output stream of invite events since they don't
	-- always occur within a room we have state for.
	sent_invite_to_output BOOLEAN NOT NULL DEFAULT FALSE,
	-- Whether the replacement has been sent to the output stream.
	sent_replaced_to_output BOOLEAN NOT NULL DEFAULT FALSE,
);

CREATE INDEX invites_active_idx ON invites (target_state_key_nid, room_nid)
	WHERE replaced_by_event_nid == 0;
`

const upsertInviteEventSQL = "" +
	"INSERT INTO invites (invite_event_nid, room_nid, target_state_key_nid," +
	" sender_state_key_nid) VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT ON invite_event_nid_unique" +
	" DO UPDATE SET sender_state_key_nid = $4"

const upsertInviteReplacedBySQL = "" +
	"INSERT INTO invites (invite_event_nid, room_nid, target_state_key_nid," +
	" replaced_by_event_nid) VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT ON invite_event_nid_unique" +
	" DO UPDATE SET replaced_by_event_nid = $4"

const selectInviteSQL = "" +
	"SELECT replaced_by_event_nid, sent_invite_to_output," +
	" sent_replaced_to_output FROM invites" +
	" WHERE invite_event_nid = $1"

const selectActiveInviteForUserInRoomSQL = "" +
	"SELECT invite_event_nid, sender_state_key_nid FROM invites" +
	" WHERE target_state_key_nid = $1 AND room_nid = $2" +
	" AND replaced_by_event_nid = 0"

const updateInviteSentInviteToOutputSQL = "" +
	"UPDATE invites SET sent_invite_to_output = TRUE" +
	" WHERE invite_event_nid = $1"

const updateInviteSentReplacedToOutputSQL = "" +
	"UPDATE invites SET sent_replaced_to_output = TRUE" +
	" WHERE invite_event_nid = $1"

type inviteStatements struct {
	upsertInviteEventStmt                *sql.Stmt
	upsertInviteReplacedByStmt           *sql.Stmt
	selectInviteStmt                     *sql.Stmt
	selectActiveInviteForUserInRoomStmt  *sql.Stmt
	updateInviteSentInviteToOutputStmt   *sql.Stmt
	updateInviteSentReplacedToOutputStmt *sql.Stmt
}

func (s *inviteStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(inviteSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.upsertInviteEventStmt, upsertInviteEventSQL},
		{&s.upsertInviteReplacedByStmt, upsertInviteReplacedBySQL},
		{&s.selectInviteStmt, selectInviteSQL},
		{&s.selectActiveInviteForUserInRoomStmt, selectActiveInviteForUserInRoomSQL},
		{&s.updateInviteSentInviteToOutputStmt, updateInviteSentInviteToOutputSQL},
		{&s.updateInviteSentReplacedToOutputStmt, updateInviteSentReplacedToOutputSQL},
	}.prepare(db)
}

func (s *inviteStatements) upsertInviteEvent(
	inviteEventNID types.EventNID, roomNID types.RoomNID,
	targetStateKeyNID, senderStateKeyNID types.EventStateKeyNID,
) error {
	_, err := s.upsertInviteEventStmt.Exec(
		inviteEventNID, roomNID, targetStateKeyNID, senderStateKeyNID,
	)
	return err
}

func (s *inviteStatements) upsertInviteReplacedBy(
	inviteEventNID types.EventNID, roomNID types.RoomNID,
	targetStateKeyNID types.EventStateKeyNID,
	replacedByEventNID types.EventNID,
) error {
	_, err := s.upsertInviteReplacedByStmt.Exec(
		inviteEventNID, roomNID, targetStateKeyNID, replacedByEventNID,
	)
	return err
}

func (s *inviteStatements) selectInvite(
	inviteEventNID types.EventNID,
) (replacedByNID types.EventNID, sentInviteToOutput, sentReplacedToOutput bool, err error) {
	err = s.selectInviteStmt.QueryRow(inviteEventNID).Scan(
		&replacedByNID, &sentInviteToOutput, &sentReplacedToOutput,
	)
	return
}

func (s *inviteStatements) selectActiveInviteForUserInRoom(
	targetStateKeyNID types.EventStateKeyNID, roomNID types.RoomNID,
) (inviteEventNID types.EventNID, senderStateKeyNID types.EventStateKeyNID, err error) {
	err = s.selectActiveInviteForUserInRoomStmt.QueryRow(
		targetStateKeyNID, roomNID,
	).Scan(&inviteEventNID, &senderStateKeyNID)
	return
}

func (s *inviteStatements) updateInviteSentInviteToOutput(
	inviteEventNID types.EventNID,
) error {
	_, err := s.updateInviteSentInviteToOutputStmt.Exec(inviteEventNID)
	return err
}

func (s *inviteStatements) updateInviteSentReplacedToOutput(
	inviteEventNID types.EventNID,
) error {
	_, err := s.updateInviteSentReplacedToOutputStmt.Exec(inviteEventNID)
	return err
}
