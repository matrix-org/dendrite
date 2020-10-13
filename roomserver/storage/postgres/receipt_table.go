// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"time"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/pkg/errors"
)

const receiptsSchema = `
-- Stores data about receipts
CREATE TABLE IF NOT EXISTS roomserver_receipts (
	-- The ID
	id INTEGER PRIMARY KEY,
	room_id TEXT NOT NULL,
	receipt_type TEXT NOT NULL,
	user_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
	receipt_ts BIGINT NOT NULL,

	CONSTRAINT roomserver_receipts_unique UNIQUE (room_id, receipt_type, user_id)
);

CREATE INDEX IF NOT EXISTS roomserver_receipts_room_id ON roomserver_receipts(room_id);
`

const upsertReceipt = "" +
	"INSERT INTO roomserver_receipts" +
	" (room_id, receipt_type, user_id, event_id, receipt_ts)" +
	" VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT (room_id, receipt_type, user_id)" +
	" DO UPDATE SET event_id = $4, receipt_ts = $5"

const selectRoomReceipts = "" +
	"SELECT id, room_id, receipt_type, user_id, event_id, receipt_ts" +
	" FROM roomserver_receipts" +
	" WHERE room_id = $1 AND receipt_ts > $2"

type receiptStatements struct {
	db                 *sql.DB
	upsertReceipt      *sql.Stmt
	selectRoomReceipts *sql.Stmt
}

func NewPostgresReceiptsTable(db *sql.DB) (tables.Receipts, error) {
	_, err := db.Exec(receiptsSchema)
	if err != nil {
		return nil, err
	}
	r := &receiptStatements{
		db: db,
	}
	if r.upsertReceipt, err = db.Prepare(upsertReceipt); err != nil {
		return nil, errors.Wrap(err, "unable to prepare upsertReceipt statement")
	}
	if r.selectRoomReceipts, err = db.Prepare(selectRoomReceipts); err != nil {
		return nil, errors.Wrap(err, "unable to prepare selectRoomReceipts statement")
	}
	return r, nil
}

func (r *receiptStatements) UpsertReceipt(ctx context.Context, txn *sql.Tx, roomId, receiptType, userId, eventId string) error {
	receiptTs := time.Now().UnixNano() / int64(time.Millisecond)
	stmt := sqlutil.TxStmt(txn, r.upsertReceipt)
	_, err := stmt.ExecContext(ctx, roomId, receiptType, userId, eventId, receiptTs)
	return err
}

func (r *receiptStatements) SelectRoomReceiptsAfter(ctx context.Context, roomId string, timestamp int) ([]types.Receipt, error) {
	rows, err := r.selectRoomReceipts.QueryContext(ctx, roomId, timestamp)
	if err != nil {
		return nil, errors.Wrap(err, "unable to query room receipts")
	}
	var res []types.Receipt
	for rows.Next() {
		r := types.Receipt{}
		err = rows.Scan(&r.ID, &r.RoomID, &r.Type, &r.UserID, &r.EventID, &r.TS)
		if err != nil {
			return res, errors.Wrap(err, "unable to scan row to api.Receipts")
		}
		res = append(res, r)
	}
	if rows.Err() != nil || rows.Close() != nil {
		return res, errors.Wrap(err, "error while scanning rows or error closing rows")
	}
	return res, nil
}
