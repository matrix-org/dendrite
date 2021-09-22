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

package cosmosdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// const receiptsSchema = `
// -- Stores data about receipts
// CREATE TABLE IF NOT EXISTS syncapi_receipts (
// 	-- The ID
// 	id BIGINT,
// 	room_id TEXT NOT NULL,
// 	receipt_type TEXT NOT NULL,
// 	user_id TEXT NOT NULL,
// 	event_id TEXT NOT NULL,
// 	receipt_ts BIGINT NOT NULL,
// 	CONSTRAINT syncapi_receipts_unique UNIQUE (room_id, receipt_type, user_id)
// );
// CREATE INDEX IF NOT EXISTS syncapi_receipts_room_id_idx ON syncapi_receipts(room_id);
// `

type receiptCosmos struct {
	ID          int64  `json:"id"`
	RoomID      string `json:"room_id"`
	ReceiptType string `json:"receipt_type"`
	UserID      string `json:"user_id"`
	EventID     string `json:"event_id"`
	ReceiptTS   int64  `json:"receipt_ts"`
}

type receiptCosmosMaxNumber struct {
	Max int64 `json:"number"`
}

type receiptCosmosData struct {
	cosmosdbapi.CosmosDocument
	Receipt receiptCosmos `json:"mx_syncapi_receipt"`
}

// const upsertReceipt = "" +
// 	"INSERT INTO syncapi_receipts" +
// 	" (id, room_id, receipt_type, user_id, event_id, receipt_ts)" +
// 	" VALUES ($1, $2, $3, $4, $5, $6)" +
// 	" ON CONFLICT (room_id, receipt_type, user_id)" +
// 	" DO UPDATE SET id = $7, event_id = $8, receipt_ts = $9"

// "SELECT id, room_id, receipt_type, user_id, event_id, receipt_ts" +
// " FROM syncapi_receipts" +
// " WHERE id > $1 and room_id in ($2)"
const selectRoomReceipts = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_receipt.id > @x2 " +
	"and ARRAY_CONTAINS(@x3, c.mx_syncapi_receipt.room_id)"

// "SELECT MAX(id) FROM syncapi_receipts"
const selectMaxReceiptIDSQL = "" +
	"select max(c.mx_syncapi_receipt.id) as number from c where c._cn = @x1 "

type receiptStatements struct {
	db                 *SyncServerDatasource
	streamIDStatements *streamIDStatements
	// upsertReceipt      *sql.Stmt
	// selectRoomReceipts *sql.Stmt
	selectMaxReceiptID string
	tableName          string
}

func (s *receiptStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *receiptStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func NewCosmosDBReceiptsTable(db *SyncServerDatasource, streamID *streamIDStatements) (tables.Receipts, error) {
	r := &receiptStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	r.selectMaxReceiptID = selectMaxReceiptIDSQL
	r.tableName = "receipts"
	return r, nil
}

// UpsertReceipt creates new user receipts
func (r *receiptStatements) UpsertReceipt(ctx context.Context, txn *sql.Tx, roomId, receiptType, userId, eventId string, timestamp gomatrixserverlib.Timestamp) (pos types.StreamPosition, err error) {
	pos, err = r.streamIDStatements.nextReceiptID(ctx, txn)
	if err != nil {
		return
	}

	// "INSERT INTO syncapi_receipts" +
	// " (id, room_id, receipt_type, user_id, event_id, receipt_ts)" +
	// " VALUES ($1, $2, $3, $4, $5, $6)" +
	// " ON CONFLICT (room_id, receipt_type, user_id)" +
	// " DO UPDATE SET id = $7, event_id = $8, receipt_ts = $9"

	// 	CONSTRAINT syncapi_receipts_unique UNIQUE (room_id, receipt_type, user_id)
	docId := fmt.Sprintf("%s_%s_%s", roomId, receiptType, userId)
	cosmosDocId := cosmosdbapi.GetDocumentId(r.db.cosmosConfig.ContainerName, r.getCollectionName(), docId)

	data := receiptCosmos{
		ID:          int64(pos),
		RoomID:      roomId,
		ReceiptType: receiptType,
		UserID:      userId,
		EventID:     eventId,
		ReceiptTS:   int64(timestamp),
	}

	var dbData = receiptCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(r.getCollectionName(), r.db.cosmosConfig.TenantName, r.getPartitionKey(), cosmosDocId),
		Receipt:        data,
	}

	var optionsCreate = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(r.db.connection).CreateDocument(
		ctx,
		r.db.cosmosConfig.DatabaseName,
		r.db.cosmosConfig.ContainerName,
		dbData,
		optionsCreate)

	// _, err = stmt.ExecContext(ctx, pos, roomId, receiptType, userId, eventId, timestamp, pos, eventId, timestamp)
	return
}

// SelectRoomReceiptsAfter select all receipts for a given room after a specific timestamp
func (r *receiptStatements) SelectRoomReceiptsAfter(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) (types.StreamPosition, []api.OutputReceiptEvent, error) {
	// "SELECT id, room_id, receipt_type, user_id, event_id, receipt_ts" +
	// " FROM syncapi_receipts" +
	// " WHERE id > $1 and room_id in ($2)"

	// selectSQL := strings.Replace(selectRoomReceipts, "($2)", sqlutil.QueryVariadicOffset(len(roomIDs), 1), 1)
	lastPos := streamPos
	// params := make([]interface{}, len(roomIDs)+1)
	// params[0] = streamPos
	// for k, v := range roomIDs {
	// 	params[k+1] = v

	params := map[string]interface{}{
		"@x1": r.getCollectionName(),
		"@x2": streamPos,
		"@x3": roomIDs,
	}
	var rows []receiptCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		r.db.connection,
		r.db.cosmosConfig.DatabaseName,
		r.db.cosmosConfig.ContainerName,
		r.getPartitionKey(), selectRoomReceipts, params, &rows)

	// rows, err := r.db.QueryContext(ctx, selectSQL, params...)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to query room receipts: %w", err)
	}

	var res []api.OutputReceiptEvent
	for _, item := range rows {
		r := api.OutputReceiptEvent{}
		var id types.StreamPosition
		// err = rows.Scan(&id, &r.RoomID, &r.Type, &r.UserID, &r.EventID, &r.Timestamp)
		// if err != nil {
		// 	return 0, res, fmt.Errorf("unable to scan row to api.Receipts: %w", err)
		// }
		id = types.StreamPosition(item.Receipt.ID)
		r.RoomID = item.Receipt.RoomID
		r.Type = item.Receipt.ReceiptType
		r.UserID = item.Receipt.UserID
		r.EventID = item.Receipt.EventID
		r.Timestamp = gomatrixserverlib.Timestamp(item.Receipt.ReceiptTS)
		res = append(res, r)
		if id > lastPos {
			lastPos = id
		}
	}
	return lastPos, res, nil
}

func (s *receiptStatements) SelectMaxReceiptID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64

	// "SELECT MAX(id) FROM syncapi_receipts"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}
	var rows []receiptCosmosMaxNumber
	err = cosmosdbapi.PerformQueryAllPartitions(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.selectMaxReceiptID, params, &rows)

	// stmt := sqlutil.TxStmt(txn, s.selectMaxReceiptID)

	if rows != nil {
		nullableID.Int64 = rows[0].Max
	}
	// err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
