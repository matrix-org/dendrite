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

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

// const peeksSchema = `
// CREATE TABLE IF NOT EXISTS syncapi_peeks (
// 	id INTEGER,
// 	room_id TEXT NOT NULL,
// 	user_id TEXT NOT NULL,
// 	device_id TEXT NOT NULL,
// 	deleted BOOL NOT NULL DEFAULT false,
//     -- When the peek was created in UNIX epoch ms.
//     creation_ts INTEGER NOT NULL,
//     UNIQUE(room_id, user_id, device_id)
// );

// CREATE INDEX IF NOT EXISTS syncapi_peeks_room_id_idx ON syncapi_peeks(room_id);
// CREATE INDEX IF NOT EXISTS syncapi_peeks_user_id_device_id_idx ON syncapi_peeks(user_id, device_id);
// `

type peekCosmos struct {
	ID       int64  `json:"id"`
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	Deleted  bool   `json:"deleted"`
	// Use the CosmosDB.Timestamp for this one
	// creation_ts    int64  `json:"creation_ts"`
}

type peekCosmosMaxNumber struct {
	Max int64 `json:"number"`
}

type peekCosmosData struct {
	cosmosdbapi.CosmosDocument
	Peek peekCosmos `json:"mx_syncapi_peek"`
}

// const insertPeekSQL = "" +
// 	"INSERT OR REPLACE INTO syncapi_peeks" +
// 	" (id, room_id, user_id, device_id, creation_ts, deleted)" +
// 	" VALUES ($1, $2, $3, $4, $5, false)"

// "UPDATE syncapi_peeks SET deleted=true, id=$1 WHERE room_id = $2 AND user_id = $3 AND device_id = $4"
const deletePeekSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_peek.room_id = @x2 " +
	"and c.mx_syncapi_peek.user_id = @x3 " +
	"and c.mx_syncapi_peek.device_id = @x4 "

// "UPDATE syncapi_peeks SET deleted=true, id=$1 WHERE room_id = $2 AND user_id = $3"
const deletePeeksSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_peek.room_id = @x2 " +
	"and c.mx_syncapi_peek.user_id = @x3 "

// we care about all the peeks which were created in this range, deleted in this range,
// or were created before this range but haven't been deleted yet.
// BEWARE: sqlite chokes on out of order substitution strings.

// "SELECT id, room_id, deleted FROM syncapi_peeks WHERE user_id = $1 AND device_id = $2 AND ((id <= $3 AND NOT deleted=true) OR (id > $3 AND id <= $4))"
const selectPeeksInRangeSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_peek.user_id = @x2 " +
	"and c.mx_syncapi_peek.device_id = @x3 " +
	"and ( " +
	"(c.mx_syncapi_peek.id <= @x4 and c.mx_syncapi_peek.deleted = false)" +
	"or " +
	"(c.mx_syncapi_peek.id > @x4 and c.mx_syncapi_peek.id <= @x5)" +
	") "

// "SELECT room_id, user_id, device_id FROM syncapi_peeks WHERE deleted=false"
const selectPeekingDevicesSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_peek.deleted = false "

// "SELECT MAX(id) FROM syncapi_peeks"
const selectMaxPeekIDSQL = "" +
	"select max(c.mx_syncapi_peek.id) from c where c._cn = @x1 "

type peekStatements struct {
	db                 *SyncServerDatasource
	streamIDStatements *streamIDStatements
	// insertPeekStmt           *sql.Stmt
	deletePeekStmt           string
	deletePeeksStmt          string
	selectPeeksInRangeStmt   string
	selectPeekingDevicesStmt string
	selectMaxPeekIDStmt      string
	tableName                string
}

func (s *peekStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *peekStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getPeek(s *peekStatements, ctx context.Context, pk string, docId string) (*peekCosmosData, error) {
	response := peekCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, nil
	}

	return &response, err
}

func NewCosmosDBPeeksTable(db *SyncServerDatasource, streamID *streamIDStatements) (tables.Peeks, error) {
	s := &peekStatements{
		db:                 db,
		streamIDStatements: streamID,
	}

	s.deletePeekStmt = deletePeekSQL
	s.deletePeeksStmt = deletePeeksSQL
	s.selectPeeksInRangeStmt = selectPeeksInRangeSQL
	s.selectPeekingDevicesStmt = selectPeekingDevicesSQL
	s.selectMaxPeekIDStmt = selectMaxPeekIDSQL
	s.tableName = "peeks"
	return s, nil
}

func (s *peekStatements) InsertPeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {
	streamPos, err = s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return
	}

	// "INSERT OR REPLACE INTO syncapi_peeks" +
	// " (id, room_id, user_id, device_id, creation_ts, deleted)" +
	// " VALUES ($1, $2, $3, $4, $5, false)"

	//     UNIQUE(room_id, user_id, device_id)
	docId := fmt.Sprintf("%s,%s,%s", roomID, userID, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getPeek(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		// " (id, room_id, user_id, device_id, creation_ts, deleted)" +
		// " VALUES ($1, $2, $3, $4, $5, false)"
		dbData.SetUpdateTime()
		dbData.Peek.Deleted = false
	} else {
		data := peekCosmos{
			ID:       int64(streamPos),
			RoomID:   roomID,
			UserID:   userID,
			DeviceID: deviceID,
		}

		dbData = &peekCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			Peek:           data,
		}
	}

	// _, err = sqlutil.TxStmt(txn, s.insertPeekStmt).ExecContext(ctx, streamPos, roomID, userID, deviceID, nowMilli)

	err = cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)

	return
}

func (s *peekStatements) DeletePeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {

	// "UPDATE syncapi_peeks SET deleted=true, id=$1 WHERE room_id = $2 AND user_id = $3 AND device_id = $4"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
		"@x3": userID,
		"@x4": deviceID,
	}

	var rows []peekCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.deletePeekStmt, params, &rows)

	// _, err = sqlutil.TxStmt(txn, s.deletePeekStmt).ExecContext(ctx, streamPos, roomID, userID, deviceID)

	numAffected := len(rows)
	if numAffected == 0 {
		return 0, cosmosdbutil.ErrNoRows
	}

	// Only create a new ID if there are rows to mark as deleted. This is handled in an SQL TX for DBs
	streamPos, err = s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return 0, err
	}

	for _, item := range rows {
		item.SetUpdateTime()
		item.Peek.Deleted = true
		item.Peek.ID = int64(streamPos)
		_, err = cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)
		if err != nil {
			return
		}
	}
	return
}

func (s *peekStatements) DeletePeeks(
	ctx context.Context, txn *sql.Tx, roomID, userID string,
) (types.StreamPosition, error) {
	// "UPDATE syncapi_peeks SET deleted=true, id=$1 WHERE room_id = $2 AND user_id = $3"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
		"@x3": userID,
	}

	var rows []peekCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.deletePeekStmt, params, &rows)

	// result, err := sqlutil.TxStmt(txn, s.deletePeeksStmt).ExecContext(ctx, streamPos, roomID, userID)
	if err != nil {
		return 0, err
	}
	numAffected := len(rows)
	if numAffected == 0 {
		return 0, cosmosdbutil.ErrNoRows
	}

	// Only create a new ID if there are rows to mark as deleted. This is handled in an SQL TX for DBs
	streamPos, err := s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return 0, err
	}

	for _, item := range rows {
		item.SetUpdateTime()
		item.Peek.Deleted = true
		item.Peek.ID = int64(streamPos)
		_, err = cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)
		if err != nil {
			return 0, err
		}
	}
	return streamPos, nil
}

func (s *peekStatements) SelectPeeksInRange(
	ctx context.Context, txn *sql.Tx, userID, deviceID string, r types.Range,
) (peeks []types.Peek, err error) {
	// "SELECT id, room_id, deleted FROM syncapi_peeks WHERE user_id = $1 AND device_id = $2 AND ((id <= $3 AND NOT deleted=true) OR (id > $3 AND id <= $4))"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": deviceID,
		"@x4": r.Low(),
		"@x5": r.High(),
	}
	var rows []peekCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectPeeksInRangeStmt, params, &rows)

	// rows, err := sqlutil.TxStmt(txn, s.selectPeeksInRangeStmt).QueryContext(ctx, userID, deviceID, r.Low(), r.High())
	if err != nil {
		return
	}

	for _, item := range rows {
		peek := types.Peek{}
		var id types.StreamPosition
		// if err = rows.Scan(&id, &peek.RoomID, &peek.Deleted); err != nil {
		// 	return
		// }
		id = types.StreamPosition(item.Peek.ID)
		peek.RoomID = item.Peek.RoomID
		peek.Deleted = item.Peek.Deleted
		peek.New = (id > r.Low() && id <= r.High()) && !peek.Deleted
		peeks = append(peeks, peek)
	}

	return peeks, nil
}

func (s *peekStatements) SelectPeekingDevices(
	ctx context.Context,
) (peekingDevices map[string][]types.PeekingDevice, err error) {

	// "SELECT room_id, user_id, device_id FROM syncapi_peeks WHERE deleted=false"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}

	var rows []peekCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectPeekingDevicesStmt, params, &rows)

	// rows, err := s.selectPeekingDevicesStmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]types.PeekingDevice)
	for _, item := range rows {
		var roomID, userID, deviceID string
		// if err := rows.Scan(&roomID, &userID, &deviceID); err != nil {
		// 	return nil, err
		// }
		roomID = item.Peek.RoomID
		userID = item.Peek.UserID
		deviceID = item.Peek.DeviceID
		devices := result[roomID]
		devices = append(devices, types.PeekingDevice{UserID: userID, DeviceID: deviceID})
		result[roomID] = devices
	}
	return result, nil
}

func (s *peekStatements) SelectMaxPeekID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	// "SELECT MAX(id) FROM syncapi_peeks"

	// stmt := sqlutil.TxStmt(txn, s.selectMaxPeekIDStmt)
	var nullableID sql.NullInt64
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}
	var rows []peekCosmosMaxNumber
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(),
		s.selectMaxPeekIDStmt, params, &rows)

	// err = stmt.QueryRowContext(ctx).Scan(&nullableID)

	if rows != nil {
		nullableID.Int64 = rows[0].Max
	}

	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
