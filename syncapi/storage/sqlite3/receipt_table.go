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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/syncapi/types"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/eduserver/api"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/pkg/errors"
)

const receiptsSchema = `
-- Stores data about receipts
CREATE TABLE IF NOT EXISTS syncapi_receipts (
	-- The ID
	id BIGINT,
	room_id TEXT NOT NULL,
	receipt_type TEXT NOT NULL,
	user_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
	receipt_ts BIGINT NOT NULL,
	CONSTRAINT syncapi_receipts_unique UNIQUE (room_id, receipt_type, user_id)
);
CREATE INDEX IF NOT EXISTS syncapi_receipts_room_id_idx ON syncapi_receipts(room_id);
`

const upsertReceipt = "" +
	"INSERT INTO syncapi_receipts" +
	" (id, room_id, receipt_type, user_id, event_id, receipt_ts)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT (room_id, receipt_type, user_id)" +
	" DO UPDATE SET id = $1, event_id = $4, receipt_ts = $5"

const selectRoomReceipts = "" +
	"SELECT room_id, receipt_type, user_id, event_id, receipt_ts" +
	" FROM syncapi_receipts" +
	" WHERE room_id = $1 AND id > $2"

type receiptStatements struct {
	db                 *sql.DB
	streamIDStatements *streamIDStatements
	upsertReceipt      *sql.Stmt
	selectRoomReceipts *sql.Stmt
}

func NewSqliteReceiptsTable(db *sql.DB, streamID *streamIDStatements) (tables.Receipts, error) {
	_, err := db.Exec(receiptsSchema)
	if err != nil {
		return nil, err
	}
	r := &receiptStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	if r.upsertReceipt, err = db.Prepare(upsertReceipt); err != nil {
		return nil, errors.Wrap(err, "unable to prepare upsertReceipt statement")
	}
	if r.selectRoomReceipts, err = db.Prepare(selectRoomReceipts); err != nil {
		return nil, errors.Wrap(err, "unable to prepare selectRoomReceipts statement")
	}
	return r, nil
}

// UpsertReceipt creates new user receipts
func (r *receiptStatements) UpsertReceipt(ctx context.Context, txn *sql.Tx, roomId, receiptType, userId, eventId string, timestamp gomatrixserverlib.Timestamp) (pos types.StreamPosition, err error) {
	pos, err = r.streamIDStatements.nextStreamID(ctx, txn)
	if err != nil {
		return
	}
	stmt := sqlutil.TxStmt(txn, r.upsertReceipt)
	_, err = stmt.ExecContext(ctx, pos, roomId, receiptType, userId, eventId, timestamp)
	return
}

// SelectRoomReceiptsAfter select all receipts for a given room after a specific timestamp
func (r *receiptStatements) SelectRoomReceiptsAfter(ctx context.Context, roomId string, streamPos types.StreamPosition) ([]api.InputReceiptEvent, error) {
	rows, err := r.selectRoomReceipts.QueryContext(ctx, roomId, streamPos)
	if err != nil {
		return []api.InputReceiptEvent{}, errors.Wrap(err, "unable to query room receipts")
	}
	var res []api.InputReceiptEvent
	for rows.Next() {
		r := api.InputReceiptEvent{}
		err = rows.Scan(&r.RoomID, &r.Type, &r.UserID, &r.EventID, &r.Timestamp)
		if err != nil {
			return res, errors.Wrap(err, "unable to scan row to types.Receipts")
		}
		res = append(res, r)
	}
	if rows.Err() != nil {
		return res, errors.Wrap(err, "error while scanning rows")
	}
	return res, rows.Close()
}
