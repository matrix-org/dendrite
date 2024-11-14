// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
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

const selectMembershipBeforeSQL = "" +
	"SELECT membership, topological_pos FROM syncapi_memberships WHERE room_id = $1 and user_id = $2 AND topological_pos <= $3 ORDER BY topological_pos DESC LIMIT 1"

const selectMembersSQL = `
SELECT event_id FROM
 ( SELECT event_id, membership FROM syncapi_memberships WHERE room_id = $1 AND topological_pos <= $2 GROUP BY user_id HAVING(max(stream_pos))) t
    WHERE ($3 IS NULL OR t.membership = $3)
		 	AND ($4 IS NULL OR t.membership <> $4)
`

const purgeMembershipsSQL = "" +
	"DELETE FROM syncapi_memberships WHERE room_id = $1"

type membershipsStatements struct {
	db                        *sql.DB
	upsertMembershipStmt      *sql.Stmt
	selectMembershipCountStmt *sql.Stmt
	//selectHeroesStmt          *sql.Stmt - prepared at runtime due to variadic
	selectMembershipForUserStmt *sql.Stmt
	selectMembersStmt           *sql.Stmt
	purgeMembershipsStmt        *sql.Stmt
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
		{&s.selectMembershipForUserStmt, selectMembershipBeforeSQL},
		{&s.selectMembersStmt, selectMembersSQL},
		{&s.purgeMembershipsStmt, purgeMembershipsSQL},
	}.Prepare(db)
}

func (s *membershipsStatements) UpsertMembership(
	ctx context.Context, txn *sql.Tx, event *rstypes.HeaderedEvent,
	streamPos, topologicalPos types.StreamPosition,
) error {
	membership, err := event.Membership()
	if err != nil {
		return fmt.Errorf("event.Membership: %w", err)
	}
	_, err = sqlutil.TxStmt(txn, s.upsertMembershipStmt).ExecContext(
		ctx,
		event.RoomID().String(),
		event.StateKeyResolved,
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

// SelectMembershipForUser returns the membership of the user before and including the given position. If no membership can be found
// returns "leave", the topological position and no error. If an error occurs, other than sql.ErrNoRows, returns that and an empty
// string as the membership.
func (s *membershipsStatements) SelectMembershipForUser(
	ctx context.Context, txn *sql.Tx, roomID, userID string, pos int64,
) (membership string, topologyPos int64, err error) {
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

func (s *membershipsStatements) PurgeMemberships(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeMembershipsStmt).ExecContext(ctx, roomID)
	return err
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
	defer internal.CloseAndLogIfError(ctx, rows, "SelectMemberships: failed to close rows")
	var eventID string
	for rows.Next() {
		if err = rows.Scan(&eventID); err != nil {
			return
		}
		eventIDs = append(eventIDs, eventID)
	}
	return eventIDs, rows.Err()
}
