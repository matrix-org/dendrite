// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/syncapi/storage/postgres/deltas"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

const receiptsSchema = `
CREATE SEQUENCE IF NOT EXISTS syncapi_receipt_id;

-- Stores data about receipts
CREATE TABLE IF NOT EXISTS syncapi_receipts (
	-- The ID
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_receipt_id'),
	room_id TEXT NOT NULL,
	receipt_type TEXT NOT NULL,
	user_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
	receipt_ts BIGINT NOT NULL,
	CONSTRAINT syncapi_receipts_unique UNIQUE (room_id, receipt_type, user_id)
);
CREATE INDEX IF NOT EXISTS syncapi_receipts_room_id ON syncapi_receipts(room_id);
`

const upsertReceipt = "" +
	"INSERT INTO syncapi_receipts" +
	" (room_id, receipt_type, user_id, event_id, receipt_ts)" +
	" VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT (room_id, receipt_type, user_id)" +
	" DO UPDATE SET id = nextval('syncapi_receipt_id'), event_id = $4, receipt_ts = $5" +
	" RETURNING id"

const selectRoomReceipts = "" +
	"SELECT id, room_id, receipt_type, user_id, event_id, receipt_ts" +
	" FROM syncapi_receipts" +
	" WHERE room_id = ANY($1) AND id > $2"

const selectMaxReceiptIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_receipts"

const purgeReceiptsSQL = "" +
	"DELETE FROM syncapi_receipts WHERE room_id = $1"

type receiptStatements struct {
	db                 *sql.DB
	upsertReceipt      *sql.Stmt
	selectRoomReceipts *sql.Stmt
	selectMaxReceiptID *sql.Stmt
	purgeReceiptsStmt  *sql.Stmt
}

func NewPostgresReceiptsTable(db *sql.DB) (tables.Receipts, error) {
	_, err := db.Exec(receiptsSchema)
	if err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrator(db)
	m.AddMigrations(sqlutil.Migration{
		Version: "syncapi: fix sequences",
		Up:      deltas.UpFixSequences,
	})
	err = m.Up(context.Background())
	if err != nil {
		return nil, err
	}
	r := &receiptStatements{
		db: db,
	}
	return r, sqlutil.StatementList{
		{&r.upsertReceipt, upsertReceipt},
		{&r.selectRoomReceipts, selectRoomReceipts},
		{&r.selectMaxReceiptID, selectMaxReceiptIDSQL},
		{&r.purgeReceiptsStmt, purgeReceiptsSQL},
	}.Prepare(db)
}

func (r *receiptStatements) UpsertReceipt(ctx context.Context, txn *sql.Tx, roomId, receiptType, userId, eventId string, timestamp spec.Timestamp) (pos types.StreamPosition, err error) {
	stmt := sqlutil.TxStmt(txn, r.upsertReceipt)
	err = stmt.QueryRowContext(ctx, roomId, receiptType, userId, eventId, timestamp).Scan(&pos)
	return
}

func (r *receiptStatements) SelectRoomReceiptsAfter(ctx context.Context, txn *sql.Tx, roomIDs []string, streamPos types.StreamPosition) (types.StreamPosition, []types.OutputReceiptEvent, error) {
	var lastPos types.StreamPosition
	rows, err := sqlutil.TxStmt(txn, r.selectRoomReceipts).QueryContext(ctx, pq.Array(roomIDs), streamPos)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to query room receipts: %w", err)
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRoomReceiptsAfter: rows.close() failed")
	var res []types.OutputReceiptEvent
	for rows.Next() {
		r := types.OutputReceiptEvent{}
		var id types.StreamPosition
		err = rows.Scan(&id, &r.RoomID, &r.Type, &r.UserID, &r.EventID, &r.Timestamp)
		if err != nil {
			return 0, res, fmt.Errorf("unable to scan row to api.Receipts: %w", err)
		}
		res = append(res, r)
		if id > lastPos {
			lastPos = id
		}
	}
	return lastPos, res, rows.Err()
}

func (s *receiptStatements) SelectMaxReceiptID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := sqlutil.TxStmt(txn, s.selectMaxReceiptID)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}

func (s *receiptStatements) PurgeReceipts(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeReceiptsStmt).ExecContext(ctx, roomID)
	return err
}
