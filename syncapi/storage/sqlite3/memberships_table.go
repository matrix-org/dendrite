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

package sqlite3

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
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
	UNIQUE (room_id, user_id, membership)
);
`

const upsertMembershipSQL = "" +
	"INSERT INTO syncapi_memberships (room_id, user_id, membership, event_id, stream_pos, topological_pos)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT (room_id, user_id, membership)" +
	" DO UPDATE SET event_id = $4, stream_pos = $5, topological_pos = $6"

const selectMembershipCountSQL = "" +
	"SELECT COUNT(*) FROM (" +
	" SELECT * FROM syncapi_memberships WHERE room_id = $1 AND stream_pos <= $2 GROUP BY user_id HAVING(max(stream_pos))" +
	") t WHERE t.membership = $3"

const selectHeroesSQL = "" +
	"SELECT DISTINCT user_id FROM syncapi_memberships WHERE room_id = $1 AND user_id != $2 AND membership IN ($3) LIMIT 5"

type membershipsStatements struct {
	db                        *sql.DB
	upsertMembershipStmt      *sql.Stmt
	selectMembershipCountStmt *sql.Stmt
	//selectHeroesStmt          *sql.Stmt - prepared at runtime due to variadic
}

func NewSqliteMembershipsTable(db *sql.DB) (tables.Memberships, error) {
	s := &membershipsStatements{
		db: db,
	}
	_, err := db.Exec(membershipsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.upsertMembershipStmt, upsertMembershipSQL},
		{&s.selectMembershipCountStmt, selectMembershipCountSQL},
		// {&s.selectHeroesStmt, selectHeroesSQL}, - prepared at runtime due to variadic
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
	stmtSQL := strings.Replace(selectHeroesSQL, "($3)", sqlutil.QueryVariadicOffset(len(memberships), 2), 1)
	stmt, err := s.db.PrepareContext(ctx, stmtSQL)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, stmt, "SelectHeroes: stmt.close() failed")
	params := []interface{}{
		roomID, userID,
	}
	for _, membership := range memberships {
		params = append(params, membership)
	}

	stmt = sqlutil.TxStmt(txn, stmt)
	var rows *sql.Rows
	rows, err = stmt.QueryContext(ctx, params...)
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
