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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/common"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

const membershipSchema = `
-- Stores data about users memberships to rooms.
CREATE TABLE IF NOT EXISTS account_memberships (
    -- The Matrix user ID localpart for the member
    localpart TEXT NOT NULL,
    -- The room this user is a member of
    room_id TEXT NOT NULL,
    -- The ID of the join membership event
    event_id TEXT NOT NULL,

    -- A user can only be member of a room once
    PRIMARY KEY (localpart, room_id)
);

-- Use index to process deletion by ID more efficiently
CREATE UNIQUE INDEX IF NOT EXISTS account_membership_event_id ON account_memberships(event_id);
`

const insertMembershipSQL = `
	INSERT INTO account_memberships(localpart, room_id, event_id) VALUES ($1, $2, $3)
	ON CONFLICT (localpart, room_id) DO UPDATE SET event_id = EXCLUDED.event_id
`

const selectMembershipsByLocalpartSQL = "" +
	"SELECT room_id, event_id FROM account_memberships WHERE localpart = $1"

const selectMembershipInRoomByLocalpartSQL = "" +
	"SELECT event_id FROM account_memberships WHERE localpart = $1 AND room_id = $2"

const selectRoomIDsByLocalPartSQL = "" +
	"SELECT room_id FROM account_memberships WHERE localpart = $1"

const deleteMembershipsByEventIDsSQL = "" +
	"DELETE FROM account_memberships WHERE event_id = ANY($1)"

type membershipStatements struct {
	deleteMembershipsByEventIDsStmt       *sql.Stmt
	insertMembershipStmt                  *sql.Stmt
	selectMembershipInRoomByLocalpartStmt *sql.Stmt
	selectMembershipsByLocalpartStmt      *sql.Stmt
	selectRoomIDsByLocalPartStmt          *sql.Stmt
}

func (s *membershipStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(membershipSchema)
	if err != nil {
		return
	}
	if s.deleteMembershipsByEventIDsStmt, err = db.Prepare(deleteMembershipsByEventIDsSQL); err != nil {
		return
	}
	if s.insertMembershipStmt, err = db.Prepare(insertMembershipSQL); err != nil {
		return
	}
	if s.selectMembershipInRoomByLocalpartStmt, err = db.Prepare(selectMembershipInRoomByLocalpartSQL); err != nil {
		return
	}
	if s.selectMembershipsByLocalpartStmt, err = db.Prepare(selectMembershipsByLocalpartSQL); err != nil {
		return
	}
	if s.selectRoomIDsByLocalPartStmt, err = db.Prepare(selectRoomIDsByLocalPartSQL); err != nil {
		return
	}
	return
}

func (s *membershipStatements) insertMembership(
	ctx context.Context, txn *sql.Tx, localpart, roomID, eventID string,
) (err error) {
	stmt := txn.Stmt(s.insertMembershipStmt)
	_, err = stmt.ExecContext(ctx, localpart, roomID, eventID)
	return
}

func (s *membershipStatements) deleteMembershipsByEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) (err error) {
	stmt := txn.Stmt(s.deleteMembershipsByEventIDsStmt)
	_, err = stmt.ExecContext(ctx, pq.StringArray(eventIDs))
	return
}

func (s *membershipStatements) selectMembershipInRoomByLocalpart(
	ctx context.Context, localpart, roomID string,
) (authtypes.Membership, error) {
	membership := authtypes.Membership{Localpart: localpart, RoomID: roomID}
	stmt := s.selectMembershipInRoomByLocalpartStmt
	err := stmt.QueryRowContext(ctx, localpart, roomID).Scan(&membership.EventID)

	return membership, err
}

func (s *membershipStatements) selectMembershipsByLocalpart(
	ctx context.Context, localpart string,
) (memberships []authtypes.Membership, err error) {
	stmt := s.selectMembershipsByLocalpartStmt
	rows, err := stmt.QueryContext(ctx, localpart)
	if err != nil {
		return
	}

	memberships = []authtypes.Membership{}

	defer common.CloseAndLogIfError(ctx, rows, "selectMembershipsByLocalpart: rows.close() failed")
	for rows.Next() {
		var m authtypes.Membership
		m.Localpart = localpart
		if err = rows.Scan(&m.RoomID, &m.EventID); err != nil {
			return
		}
		memberships = append(memberships, m)
	}
	return memberships, rows.Err()
}

func (s *membershipStatements) selectRoomIDsByLocalPart(
	ctx context.Context, localPart string,
) ([]string, error) {
	stmt := s.selectRoomIDsByLocalPartStmt
	rows, err := stmt.QueryContext(ctx, localPart)
	if err != nil {
		return nil, err
	}
	roomIDs := []string{}
	defer rows.Close() // nolint: errcheck
	for rows.Next() {
		var roomID string
		if err = rows.Scan(&roomID); err != nil {
			return nil, err
		}
		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, rows.Err()
}
