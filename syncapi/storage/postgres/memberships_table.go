// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

// The memberships table is designed to track the last time that
// the user was a given state. This allows us to find out the
// most recent time that a user was invited to, joined or left
// a room, either by choice or otherwise. This is important for
// building history visibility.

const membershipsSchema = `
CREATE TABLE IF NOT EXISTS syncapi_memberships (
    -- The 'room_id' key for the state event.
    room_id TEXT NOT NULL,
    -- The state event ID
	user_id TEXT NOT NULL,
	-- The status of the membership
	membership TEXT NOT NULL,
	-- The event ID that last changed the membership
	event_id TEXT NOT NULL,
	-- The stream position of the change
	stream_pos BIGINT NOT NULL,
	-- The topological position of the change in the room
	topological_pos BIGINT NOT NULL,
	-- Unique index
	CONSTRAINT syncapi_memberships_unique UNIQUE (room_id, user_id, membership)
);
`

const upsertMembershipSQL = "" +
	"INSERT INTO syncapi_memberships (room_id, user_id, membership, event_id, stream_pos, topological_pos)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT ON CONSTRAINT syncapi_memberships_unique" +
	" DO UPDATE SET event_id = $4, stream_pos = $5, topological_pos = $6"

const selectMembershipCountSQL = "" +
	"SELECT COUNT(*) FROM (" +
	" SELECT DISTINCT ON (room_id, user_id) room_id, user_id, membership FROM syncapi_memberships WHERE room_id = $1 AND stream_pos <= $2 ORDER BY room_id, user_id, stream_pos DESC" +
	") t WHERE t.membership = $3"

const selectHeroesSQL = "" +
	"SELECT DISTINCT user_id FROM syncapi_memberships WHERE room_id = $1 AND user_id != $2 AND membership = ANY($3) LIMIT 5"

const selectMembershipBeforeSQL = "" +
	"SELECT membership, topological_pos FROM syncapi_memberships WHERE room_id = $1 and user_id = $2 AND topological_pos <= $3 ORDER BY topological_pos DESC LIMIT 1"

const selectMembersSQL = `
SELECT event_id FROM (
	SELECT DISTINCT ON (room_id, user_id) room_id, user_id, event_id, membership FROM syncapi_memberships WHERE room_id = $1 AND topological_pos <= $2 ORDER BY room_id, user_id, stream_pos DESC  
) t 
WHERE ($3::text IS NULL OR t.membership = $3)
	AND ($4::text IS NULL OR t.membership <> $4)
`

type membershipsStatements struct {
	upsertMembershipStmt        *sql.Stmt
	selectMembershipCountStmt   *sql.Stmt
	selectHeroesStmt            *sql.Stmt
	selectMembershipForUserStmt *sql.Stmt
	selectMembersStmt           *sql.Stmt
}

func NewPostgresMembershipsTable(db *sql.DB) (tables.Memberships, error) {
	s := &membershipsStatements{}
	_, err := db.Exec(membershipsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.upsertMembershipStmt, upsertMembershipSQL},
		{&s.selectMembershipCountStmt, selectMembershipCountSQL},
		{&s.selectHeroesStmt, selectHeroesSQL},
		{&s.selectMembershipForUserStmt, selectMembershipBeforeSQL},
		{&s.selectMembersStmt, selectMembersSQL},
	}.Prepare(db)
}

func (s *membershipsStatements) UpsertMembership(
	ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.HeaderedEvent,
	streamPos, topologicalPos types.StreamPosition,
) error {
	membership, err := event.Membership()
	if err != nil {
		return fmt.Errorf("event.Membership: %w", err)
	}
	_, err = sqlutil.TxStmt(txn, s.upsertMembershipStmt).ExecContext(
		ctx,
		event.RoomID(),
		*event.StateKey(),
		membership,
		event.EventID(),
		streamPos,
		topologicalPos,
	)
	return err
}

func (s *membershipsStatements) SelectMembershipCount(
	ctx context.Context, txn *sql.Tx, roomID, membership string, pos types.StreamPosition,
) (count int, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectMembershipCountStmt)
	err = stmt.QueryRowContext(ctx, roomID, pos, membership).Scan(&count)
	return
}

func (s *membershipsStatements) SelectHeroes(
	ctx context.Context, txn *sql.Tx, roomID, userID string, memberships []string,
) (heroes []string, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectHeroesStmt)
	var rows *sql.Rows
	rows, err = stmt.QueryContext(ctx, roomID, userID, pq.StringArray(memberships))
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectHeroes: rows.close() failed")
	var hero string
	for rows.Next() {
		if err = rows.Scan(&hero); err != nil {
			return
		}
		heroes = append(heroes, hero)
	}
	return heroes, rows.Err()
}

// SelectMembershipForUser returns the membership of the user before and including the given position. If no membership can be found
// returns "leave", the topological position and no error. If an error occurs, other than sql.ErrNoRows, returns that and an empty
// string as the membership.
func (s *membershipsStatements) SelectMembershipForUser(
	ctx context.Context, txn *sql.Tx, roomID, userID string, pos int64,
) (membership string, topologyPos int, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectMembershipForUserStmt)
	err = stmt.QueryRowContext(ctx, roomID, userID, pos).Scan(&membership, &topologyPos)
	if err != nil {
		if err == sql.ErrNoRows {
			return "leave", 0, nil
		}
		return "", 0, err
	}
	return membership, topologyPos, nil
}

func (s *membershipsStatements) SelectMemberships(
	ctx context.Context, txn *sql.Tx,
	roomID string, pos types.TopologyToken,
	membership, notMembership *string,
) (eventIDs []string, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectMembersStmt)
	rows, err := stmt.QueryContext(ctx, roomID, pos.Depth, membership, notMembership)
	if err != nil {
		return
	}
	var (
		eventID string
	)
	for rows.Next() {
		if err = rows.Scan(&eventID); err != nil {
			return
		}
		eventIDs = append(eventIDs, eventID)
	}
	return eventIDs, rows.Err()
}
