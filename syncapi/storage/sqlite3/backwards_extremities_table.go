// Copyright 2018-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
)

const backwardExtremitiesSchema = `
-- Stores output room events received from the roomserver.
CREATE TABLE IF NOT EXISTS syncapi_backward_extremities (
	-- The 'room_id' key for the event.
	room_id TEXT NOT NULL,
	-- The event ID for the last known event. This is the backwards extremity.
	event_id TEXT NOT NULL,
	-- The prev_events for the last known event. This is used to update extremities.
	prev_event_id TEXT NOT NULL,
	PRIMARY KEY(room_id, event_id, prev_event_id)
);
`

const insertBackwardExtremitySQL = "" +
	"INSERT INTO syncapi_backward_extremities (room_id, event_id, prev_event_id)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT (room_id, event_id, prev_event_id) DO NOTHING"

const selectBackwardExtremitiesForRoomSQL = "" +
	"SELECT event_id, prev_event_id FROM syncapi_backward_extremities WHERE room_id = $1"

const deleteBackwardExtremitySQL = "" +
	"DELETE FROM syncapi_backward_extremities WHERE room_id = $1 AND prev_event_id = $2"

const purgeBackwardExtremitiesSQL = "" +
	"DELETE FROM syncapi_backward_extremities WHERE room_id = $1"

type backwardExtremitiesStatements struct {
	db                                   *sql.DB
	insertBackwardExtremityStmt          *sql.Stmt
	selectBackwardExtremitiesForRoomStmt *sql.Stmt
	deleteBackwardExtremityStmt          *sql.Stmt
	purgeBackwardExtremitiesStmt         *sql.Stmt
}

func NewSqliteBackwardsExtremitiesTable(db *sql.DB) (tables.BackwardsExtremities, error) {
	s := &backwardExtremitiesStatements{
		db: db,
	}
	_, err := db.Exec(backwardExtremitiesSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertBackwardExtremityStmt, insertBackwardExtremitySQL},
		{&s.selectBackwardExtremitiesForRoomStmt, selectBackwardExtremitiesForRoomSQL},
		{&s.deleteBackwardExtremityStmt, deleteBackwardExtremitySQL},
		{&s.purgeBackwardExtremitiesStmt, purgeBackwardExtremitiesSQL},
	}.Prepare(db)
}

func (s *backwardExtremitiesStatements) InsertsBackwardExtremity(
	ctx context.Context, txn *sql.Tx, roomID, eventID string, prevEventID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.insertBackwardExtremityStmt).ExecContext(ctx, roomID, eventID, prevEventID)
	return err
}

func (s *backwardExtremitiesStatements) SelectBackwardExtremitiesForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) (bwExtrems map[string][]string, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectBackwardExtremitiesForRoomStmt).QueryContext(ctx, roomID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectBackwardExtremitiesForRoom: rows.close() failed")

	bwExtrems = make(map[string][]string)
	for rows.Next() {
		var eID string
		var prevEventID string
		if err = rows.Scan(&eID, &prevEventID); err != nil {
			return
		}
		bwExtrems[eID] = append(bwExtrems[eID], prevEventID)
	}

	return bwExtrems, rows.Err()
}

func (s *backwardExtremitiesStatements) DeleteBackwardExtremity(
	ctx context.Context, txn *sql.Tx, roomID, knownEventID string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteBackwardExtremityStmt).ExecContext(ctx, roomID, knownEventID)
	return err
}

func (s *backwardExtremitiesStatements) PurgeBackwardExtremities(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeBackwardExtremitiesStmt).ExecContext(ctx, roomID)
	return err
}
