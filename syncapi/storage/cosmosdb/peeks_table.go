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
	"time"

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

type PeekCosmos struct {
	ID       int64  `json:"id"`
	RoomID   string `json:"room_id"`
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	Deleted  bool   `json:"deleted"`
	// Use the CosmosDB.Timestamp for this one
	// creation_ts    int64  `json:"creation_ts"`
}

type PeekCosmosMaxNumber struct {
	Max int64 `json:"number"`
}

type PeekCosmosData struct {
	Id        string     `json:"id"`
	Pk        string     `json:"_pk"`
	Cn        string     `json:"_cn"`
	ETag      string     `json:"_etag"`
	Timestamp int64      `json:"_ts"`
	Peek      PeekCosmos `json:"mx_syncapi_peek"`
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

func queryPeek(s *peekStatements, ctx context.Context, qry string, params map[string]interface{}) ([]PeekCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []PeekCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func queryPeekMaxNumber(s *peekStatements, ctx context.Context, qry string, params map[string]interface{}) ([]PeekCosmosMaxNumber, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []PeekCosmosMaxNumber

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, nil
	}
	return response, nil
}

func setPeek(s *peekStatements, ctx context.Context, peek PeekCosmosData) (*PeekCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(peek.Pk, peek.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		peek.Id,
		&peek,
		optionsReplace)
	return &peek, ex
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     UNIQUE(room_id, user_id, device_id)
	docId := fmt.Sprintf("%d_%s_%d", roomID, userID, deviceID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	data := PeekCosmos{
		ID:       int64(streamPos),
		RoomID:   roomID,
		UserID:   userID,
		DeviceID: deviceID,
	}

	dbData := &PeekCosmosData{
		Id: cosmosDocId,
		Cn: dbCollectionName,
		Pk: pk,
		// nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
		Timestamp: time.Now().Unix(),
		Peek:      data,
	}

	// _, err = sqlutil.TxStmt(txn, s.insertPeekStmt).ExecContext(ctx, streamPos, roomID, userID, deviceID, nowMilli)

	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return
}

func (s *peekStatements) DeletePeek(
	ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string,
) (streamPos types.StreamPosition, err error) {

	// "UPDATE syncapi_peeks SET deleted=true, id=$1 WHERE room_id = $2 AND user_id = $3 AND device_id = $4"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
		"@x3": userID,
		"@x4": deviceID,
	}

	rows, err := queryPeek(s, ctx, s.deletePeekStmt, params)
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
		item.Peek.Deleted = true
		item.Peek.ID = int64(streamPos)
		_, err = setPeek(s, ctx, item)
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
		"@x3": userID,
	}

	rows, err := queryPeek(s, ctx, s.deletePeekStmt, params)
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
		item.Peek.Deleted = true
		item.Peek.ID = int64(streamPos)
		_, err = setPeek(s, ctx, item)
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
		"@x3": deviceID,
		"@x4": r.Low(),
		"@x5": r.High(),
	}

	rows, err := queryPeek(s, ctx, s.selectPeeksInRangeStmt, params)
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	rows, err := queryPeek(s, ctx, s.selectPeekingDevicesStmt, params)
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	rows, err := queryPeekMaxNumber(s, ctx, s.selectMaxPeekIDStmt, params)
	// err = stmt.QueryRowContext(ctx).Scan(&nullableID)

	if rows != nil {
		nullableID.Int64 = rows[0].Max
	}

	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
