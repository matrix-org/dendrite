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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

// var deviceKeysSchema = `
// -- Stores device keys for users
// CREATE TABLE IF NOT EXISTS keyserver_device_keys (
//     user_id TEXT NOT NULL,
// 	device_id TEXT NOT NULL,
// 	ts_added_secs BIGINT NOT NULL,
// 	key_json TEXT NOT NULL,
// 	stream_id BIGINT NOT NULL,
// 	display_name TEXT,
// 	-- Clobber based on tuple of user/device.
//     UNIQUE (user_id, device_id)
// );
// `

type DeviceKeyCosmos struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	// Use the CosmosDB.Timestamp for this one
	// TSAddedSecs int64  `json:"ts_added_secs"`
	KeyJSON     []byte `json:"key_json"`
	StreamID    int    `json:"stream_id"`
	DisplayName string `json:"display_name"`
}

type DeviceKeyCosmosNumber struct {
	Number int64 `json:"number"`
}

type DeviceKeyCosmosData struct {
	Id        string          `json:"id"`
	Pk        string          `json:"_pk"`
	Cn        string          `json:"_cn"`
	ETag      string          `json:"_etag"`
	Timestamp int64           `json:"_ts"`
	DeviceKey DeviceKeyCosmos `json:"mx_keyserver_device_key"`
}

// const upsertDeviceKeysSQL = "" +
// 	"INSERT INTO keyserver_device_keys (user_id, device_id, ts_added_secs, key_json, stream_id, display_name)" +
// 	" VALUES ($1, $2, $3, $4, $5, $6)" +
// 	" ON CONFLICT (user_id, device_id)" +
// 	" DO UPDATE SET key_json = $4, stream_id = $5, display_name = $6"

// const selectDeviceKeysSQL = "" +
// 	"SELECT key_json, stream_id, display_name FROM keyserver_device_keys WHERE user_id=$1 AND device_id=$2"

// "SELECT device_id, key_json, stream_id, display_name FROM keyserver_device_keys WHERE user_id=$1 AND key_json <> ''"
const selectBatchDeviceKeysSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_device_key.user_id = @x2 " +
	"and c.mx_keyserver_device_key.key_json <> \"\""

// "SELECT MAX(stream_id) FROM keyserver_device_keys WHERE user_id=$1"
const selectMaxStreamForUserSQL = "" +
	"select max(c.mx_keyserver_device_key.stream_id) as number from c where c._cn = @x1 " +
	"and c.mx_keyserver_device_key.user_id = @x2 "

// "SELECT COUNT(*) FROM keyserver_device_keys WHERE user_id=$1 AND stream_id IN ($2)"
const countStreamIDsForUserSQL = "" +
	"select count(c._ts) as number from c where c._cn = @x1 " +
	"and c.mx_keyserver_device_key.user_id = @x2 " +
	"and ARRAY_CONTAINS(@x3, c.mx_keyserver_device_key.stream_id) "

const selectAllDeviceKeysSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_device_key.user_id = @x2 "

// const deleteAllDeviceKeysSQL = "" +
// 	"DELETE FROM keyserver_device_keys WHERE user_id=$1"

func queryDeviceKey(s *deviceKeysStatements, ctx context.Context, qry string, params map[string]interface{}) ([]DeviceKeyCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []DeviceKeyCosmosData

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

func queryDeviceKeyNumber(s *deviceKeysStatements, ctx context.Context, qry string, params map[string]interface{}) ([]DeviceKeyCosmosNumber, error) {
	var response []DeviceKeyCosmosNumber

	var optionsQry = cosmosdbapi.GetQueryAllPartitionsDocumentsOptions()
	var query = cosmosdbapi.GetQuery(qry, params)
	var _, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}

	if len(response) == 0 {
		return nil, cosmosdbutil.ErrNoRows
	}

	return response, nil
}

func getDeviceKey(s *deviceKeysStatements, ctx context.Context, pk string, docId string) (*DeviceKeyCosmosData, error) {
	response := DeviceKeyCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response, err
}

func insertDeviceKeyCore(s *deviceKeysStatements, ctx context.Context, dbData DeviceKeyCosmosData) error {
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	if err != nil {
		return err
	}

	return nil
}

func mapFromDeviceKeyMessage(key api.DeviceMessage) DeviceKeyCosmos {
	return DeviceKeyCosmos{
		DeviceID:    key.DeviceID,
		DisplayName: key.DisplayName,
		KeyJSON:     key.KeyJSON,
		StreamID:    key.StreamID,
		UserID:      key.UserID,
	}
}

type deviceKeysStatements struct {
	db *Database
	// upsertDeviceKeysStmt       *sql.Stmt
	// selectDeviceKeysStmt       *sql.Stmt
	selectBatchDeviceKeysStmt  string
	selectMaxStreamForUserStmt string
	// deleteAllDeviceKeysStmt    *sql.Stmt
	tableName string
}

func NewCosmosDBDeviceKeysTable(db *Database) (tables.DeviceKeys, error) {
	s := &deviceKeysStatements{
		db: db,
	}
	s.selectBatchDeviceKeysStmt = selectBatchDeviceKeysSQL
	s.selectMaxStreamForUserStmt = selectMaxStreamForUserSQL
	s.tableName = "device_keys"
	return s, nil
}

func deleteDeviceKeyCore(s *deviceKeysStatements, ctx context.Context, dbData DeviceKeyCosmosData) error {
	var options = cosmosdbapi.GetDeleteDocumentOptions(dbData.Pk)
	var _, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Id,
		options)

	if err != nil {
		return err
	}
	return err
}

func (s *deviceKeysStatements) DeleteAllDeviceKeys(ctx context.Context, txn *sql.Tx, userID string) error {

	// 	"DELETE FROM keyserver_device_keys WHERE user_id=$1"
	// _, err := sqlutil.TxStmt(txn, s.deleteAllDeviceKeysStmt).ExecContext(ctx, userID)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
	}
	response, err := queryDeviceKey(s, ctx, selectAllDeviceKeysSQL, params)

	if err != nil {
		return err
	}

	for _, item := range response {
		errItem := deleteDeviceKeyCore(s, ctx, item)
		if errItem != nil {
			return errItem
		}
	}
	return nil
}

func (s *deviceKeysStatements) SelectBatchDeviceKeys(ctx context.Context, userID string, deviceIDs []string) ([]api.DeviceMessage, error) {
	deviceIDMap := make(map[string]bool)

	// "SELECT device_id, key_json, stream_id, display_name FROM keyserver_device_keys WHERE user_id=$1 AND key_json <> ''"

	for _, d := range deviceIDs {
		deviceIDMap[d] = true
	}
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
	}
	response, err := queryDeviceKey(s, ctx, s.selectBatchDeviceKeysStmt, params)
	// rows, err := s.selectBatchDeviceKeysStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	// defer internal.CloseAndLogIfError(ctx, rows, "selectBatchDeviceKeysStmt: rows.close() failed")

	var result []api.DeviceMessage
	for _, item := range response {
		var dk api.DeviceMessage
		dk.UserID = userID
		// var keyJSON string
		var streamID int
		// var displayName sql.NullString
		// if err := rows.Scan(&dk.DeviceID, &keyJSON, &streamID, &displayName); err != nil {
		// 	return nil, err
		// }
		streamID = item.DeviceKey.StreamID

		dk.KeyJSON = item.DeviceKey.KeyJSON
		dk.StreamID = streamID
		if len(item.DeviceKey.DisplayName) > 0 {
			dk.DisplayName = item.DeviceKey.DisplayName
		}
		// include the key if we want all keys (no device) or it was asked
		if deviceIDMap[dk.DeviceID] || len(deviceIDs) == 0 {
			result = append(result, dk)
		}
	}
	return result, nil
}

func (s *deviceKeysStatements) SelectDeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error {
	for i, key := range keys {
		var keyJSON []byte
		var streamID int
		var displayName sql.NullString

		// 	"SELECT key_json, stream_id, display_name FROM keyserver_device_keys WHERE user_id=$1 AND device_id=$2"

		// err := s.selectDeviceKeysStmt.QueryRowContext(ctx, key.UserID, key.DeviceID).Scan(&keyJSONStr, &streamID, &displayName)
		var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
		//     UNIQUE (user_id, device_id)
		docId := fmt.Sprintf("%s_%s", key.UserID, key.DeviceID)
		cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
		pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

		response, err := getDeviceKey(s, ctx, pk, cosmosDocId)

		if err != nil && err != cosmosdbutil.ErrNoRows {
			return err
		}
		if response != nil {
			keyJSON = response.DeviceKey.KeyJSON
			streamID = response.DeviceKey.StreamID
			displayName.String = response.DeviceKey.DisplayName
		}

		// this will be '' when there is no device
		keys[i].KeyJSON = keyJSON
		keys[i].StreamID = streamID
		if displayName.Valid {
			keys[i].DisplayName = displayName.String
		}
	}
	return nil
}

func (s *deviceKeysStatements) SelectMaxStreamIDForUser(ctx context.Context, txn *sql.Tx, userID string) (streamID int32, err error) {
	// nullable if there are no results
	var nullStream sql.NullInt32

	// "SELECT MAX(stream_id) FROM keyserver_device_keys WHERE user_id=$1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
	}

	// err = sqlutil.TxStmt(txn, s.selectMaxStreamForUserStmt).QueryRowContext(ctx, userID).Scan(&nullStream)
	response, err := queryDeviceKeyNumber(s, ctx, countStreamIDsForUserSQL, params)

	if err != nil {
		if err == cosmosdbutil.ErrNoRows {
			err = nil
		} else {
			return nullStream.Int32, err
		}
	}

	if len(response) > 0 {
		nullStream.Int32 = int32(response[0].Number)
	}

	if nullStream.Valid {
		streamID = nullStream.Int32
	}
	return
}

func (s *deviceKeysStatements) CountStreamIDsForUser(ctx context.Context, userID string, streamIDs []int64) (int, error) {

	// "SELECT COUNT(*) FROM keyserver_device_keys WHERE user_id=$1 AND stream_id IN ($2)"

	iStreamIDs := make([]interface{}, len(streamIDs)+1)
	iStreamIDs[0] = userID
	for i := range streamIDs {
		iStreamIDs[i+1] = streamIDs[i]
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
		"@x3": iStreamIDs,
	}

	// query := strings.Replace(countStreamIDsForUserSQL, "($2)", sqlutil.QueryVariadicOffset(len(streamIDs), 1), 1)
	// // nullable if there are no results
	// var count sql.NullInt32
	// err := s.db.QueryRowContext(ctx, query, iStreamIDs...).Scan(&count)

	response, err := queryDeviceKeyNumber(s, ctx, countStreamIDsForUserSQL, params)

	if err != nil {
		return 0, err
	}
	// if count.Valid {
	// 	return int(count.Int32), nil
	// }
	if response[0].Number >= 0 {
		return int(response[0].Number), nil
	}
	return 0, nil
}

func (s *deviceKeysStatements) InsertDeviceKeys(ctx context.Context, txn *sql.Tx, keys []api.DeviceMessage) error {

	// "INSERT INTO keyserver_device_keys (user_id, device_id, ts_added_secs, key_json, stream_id, display_name)" +
	// " VALUES ($1, $2, $3, $4, $5, $6)" +
	// " ON CONFLICT (user_id, device_id)" +
	// " DO UPDATE SET key_json = $4, stream_id = $5, display_name = $6"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	for _, key := range keys {
		now := time.Now().Unix()
		//     UNIQUE (user_id, device_id)
		docId := fmt.Sprintf("%s_%s", key.UserID, key.DeviceID)
		cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)

		dbData := &DeviceKeyCosmosData{
			Id:        cosmosDocId,
			Cn:        dbCollectionName,
			Pk:        pk,
			Timestamp: now,
			DeviceKey: mapFromDeviceKeyMessage(key),
		}

		err := insertDeviceKeyCore(s, ctx, *dbData)

		if err != nil {
			return err
		}
	}
	return nil
}
