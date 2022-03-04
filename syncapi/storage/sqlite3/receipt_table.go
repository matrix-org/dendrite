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
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
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
	" DO UPDATE SET id = $7, event_id = $8, receipt_ts = $9"

const selectRoomReceipts = "" +
	"SELECT id, room_id, receipt_type, user_id, event_id, receipt_ts" +
	" FROM syncapi_receipts" +
	" WHERE id > $1 and room_id in ($2)"

const selectMaxReceiptIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_receipts"

type receiptStatements struct {
	db                 *sql.DB
	streamIDStatements *streamIDStatements
	upsertReceipt      *sql.Stmt
	selectRoomReceipts *sql.Stmt
	selectMaxReceiptID *sql.Stmt
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
		return nil, fmt.Errorf("unable to prepare upsertReceipt statement: %w", err)
	}
	if r.selectRoomReceipts, err = db.Prepare(selectRoomReceipts); err != nil {
		return nil, fmt.Errorf("unable to prepare selectRoomReceipts statement: %w", err)
	}
	if r.selectMaxReceiptID, err = db.Prepare(selectMaxReceiptIDSQL); err != nil {
		return nil, fmt.Errorf("unable to prepare selectRoomReceipts statement: %w", err)
	}
	return r, nil
}

// UpsertReceipt creates new user receipts
func (r *receiptStatements) UpsertReceipt(ctx context.Context, txn *sql.Tx, roomId, receiptType, userId, eventId string, timestamp gomatrixserverlib.Timestamp) (pos types.StreamPosition, err error) {
	pos, err = r.streamIDStatements.nextReceiptID(ctx, txn)
	if err != nil {
		return
	}
	stmt := sqlutil.TxStmt(txn, r.upsertReceipt)
	_, err = stmt.ExecContext(ctx, pos, roomId, receiptType, userId, eventId, timestamp, pos, eventId, timestamp)
	return
}

// SelectRoomReceiptsAfter select all receipts for a given room after a specific timestamp
func (r *receiptStatements) SelectRoomReceiptsAfter(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) (types.StreamPosition, []api.OutputReceiptEvent, error) {
	selectSQL := strings.Replace(selectRoomReceipts, "($2)", sqlutil.QueryVariadicOffset(len(roomIDs), 1), 1)
	var lastPos types.StreamPosition
	params := make([]interface{}, len(roomIDs)+1)
	params[0] = streamPos
	for k, v := range roomIDs {
		params[k+1] = v
	}
	rows, err := r.db.QueryContext(ctx, selectSQL, params...)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to query room receipts: %w", err)
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectRoomReceiptsAfter: rows.close() failed")
	var res []api.OutputReceiptEvent
	for rows.Next() {
		r := api.OutputReceiptEvent{}
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
