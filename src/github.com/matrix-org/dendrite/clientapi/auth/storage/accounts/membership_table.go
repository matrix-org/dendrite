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

package accounts

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

const insertMembershipSQL = `
	INSERT INTO account_memberships(localpart, room_id, event_id) VALUES ($1, $2, $3)
	ON CONFLICT (localpart, room_id) DO UPDATE SET event_id = EXCLUDED.event_id
`

const selectMembershipsByLocalpartSQL = "" +
	"SELECT room_id, event_id FROM account_memberships WHERE localpart = $1"

const deleteMembershipsByEventIDsSQL = "" +
	"DELETE FROM account_memberships WHERE event_id = ANY($1)"

type membershipStatements struct {
	deleteMembershipsByEventIDsStmt  *sql.Stmt
	insertMembershipStmt             *sql.Stmt
	selectMembershipsByLocalpartStmt *sql.Stmt
}

func (s *membershipStatements) prepare(db *sql.DB) (err error) {
	if s.deleteMembershipsByEventIDsStmt, err = db.Prepare(deleteMembershipsByEventIDsSQL); err != nil {
		return
	}
	if s.insertMembershipStmt, err = db.Prepare(insertMembershipSQL); err != nil {
		return
	}
	if s.selectMembershipsByLocalpartStmt, err = db.Prepare(selectMembershipsByLocalpartSQL); err != nil {
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

func (s *membershipStatements) selectMembershipsByLocalpart(
	ctx context.Context, localpart string,
) (memberships []authtypes.Membership, err error) {
	stmt := s.selectMembershipsByLocalpartStmt
	rows, err := stmt.QueryContext(ctx, localpart)
	if err != nil {
		return
	}

	memberships = []authtypes.Membership{}

	defer rows.Close() // nolint: errcheck
	for rows.Next() {
		var m authtypes.Membership
		m.Localpart = localpart
		if err := rows.Scan(&m.RoomID, &m.EventID); err != nil {
			return nil, err
		}
		memberships = append(memberships, m)
	}

	return
}
