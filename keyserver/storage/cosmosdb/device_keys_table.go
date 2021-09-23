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

type deviceKeyCosmos struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	// Use the CosmosDB.Timestamp for this one
	// TSAddedSecs int64  `json:"ts_added_secs"`
	KeyJSON     []byte `json:"key_json"`
	StreamID    int    `json:"stream_id"`
	DisplayName string `json:"display_name"`
}

type deviceKeyCosmosNumber struct {
	Number int64 `json:"number"`
}

type deviceKeyCosmosData struct {
	cosmosdbapi.CosmosDocument
	DeviceKey deviceKeyCosmos `json:"mx_keyserver_device_key"`
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
	"select max(c.mx_keyserver_device_key.stream_id) as number from c where c._sid = @x1 and c._cn = @x2 " +
	"and c.mx_keyserver_device_key.user_id = @x3 "

// "SELECT COUNT(*) FROM keyserver_device_keys WHERE user_id=$1 AND stream_id IN ($2)"
const countStreamIDsForUserSQL = "" +
	"select count(c._ts) as number from c where c._sid = @x1 and c._cn = @x2 " +
	"and c.mx_keyserver_device_key.user_id = @x3 " +
	"and ARRAY_CONTAINS(@x4, c.mx_keyserver_device_key.stream_id) "

const selectAllDeviceKeysSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_device_key.user_id = @x2 "

// "DELETE FROM keyserver_device_keys WHERE user_id=$1 AND device_id=$2"
const deleteDeviceKeysSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_device_key.user_id = @x2 " +
	"and c.mx_keyserver_device_key.device_id = @x3 "

	// const deleteAllDeviceKeysSQL = "" +
// 	"DELETE FROM keyserver_device_keys WHERE user_id=$1"

func getDeviceKey(s *deviceKeysStatements, ctx context.Context, pk string, docId string) (*deviceKeyCosmosData, error) {
	response := deviceKeyCosmosData{}
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

func insertDeviceKeyCore(s *deviceKeysStatements, ctx context.Context, dbData deviceKeyCosmosData) error {
	// "INSERT INTO keyserver_device_keys (user_id, device_id, ts_added_secs, key_json, stream_id, display_name)" +
	// " VALUES ($1, $2, $3, $4, $5, $6)" +
	// " ON CONFLICT (user_id, device_id)" +
	// " DO UPDATE SET key_json = $4, stream_id = $5, display_name = $6"
	existing, _ := getDeviceKey(s, ctx, dbData.Pk, dbData.Id)
	if existing != nil {
		existing.SetUpdateTime()
		existing.DeviceKey.KeyJSON = dbData.DeviceKey.KeyJSON
		existing.DeviceKey.StreamID = dbData.DeviceKey.StreamID
		existing.DeviceKey.DisplayName = dbData.DeviceKey.DisplayName
	} else {
		existing = &dbData
	}

	err := cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		existing.Pk,
		existing)

	if err != nil {
		return err
	}

	return nil
}

func mapFromDeviceKeyMessage(key api.DeviceMessage) deviceKeyCosmos {
	return deviceKeyCosmos{
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
	deleteDeviceKeysStmt       string
	// deleteAllDeviceKeysStmt    *sql.Stmt
	tableName string
}

func (s *deviceKeysStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *deviceKeysStatements) getPartitionKey(userId string) string {
	uniqueId := userId
	return cosmosdbapi.GetPartitionKeyByUniqueId(s.db.cosmosConfig.TenantName, s.getCollectionName(), uniqueId)
}

func NewCosmosDBDeviceKeysTable(db *Database) (tables.DeviceKeys, error) {
	s := &deviceKeysStatements{
		db: db,
	}
	s.selectBatchDeviceKeysStmt = selectBatchDeviceKeysSQL
	s.selectMaxStreamForUserStmt = selectMaxStreamForUserSQL
	s.deleteDeviceKeysStmt = deleteDeviceKeysSQL
	s.tableName = "device_keys"
	return s, nil
}

func deleteDeviceKeyCore(s *deviceKeysStatements, ctx context.Context, dbData deviceKeyCosmosData) error {
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

func (s *deviceKeysStatements) DeleteDeviceKeys(ctx context.Context, txn *sql.Tx, userID, deviceID string) error {
	// "DELETE FROM keyserver_device_keys WHERE user_id=$1 AND device_id=$2"
	// _, err := sqlutil.TxStmt(txn, s.deleteDeviceKeysStmt).ExecContext(ctx, userID, deviceID)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": deviceID,
	}

	var rows []deviceKeyCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), selectAllDeviceKeysSQL, params, &rows)

	if err != nil {
		return err
	}

	for _, item := range rows {
		errItem := deleteDeviceKeyCore(s, ctx, item)
		if errItem != nil {
			return errItem
		}
	}
	return nil
}

func (s *deviceKeysStatements) DeleteAllDeviceKeys(ctx context.Context, txn *sql.Tx, userID string) error {

	// 	"DELETE FROM keyserver_device_keys WHERE user_id=$1"
	// _, err := sqlutil.TxStmt(txn, s.deleteAllDeviceKeysStmt).ExecContext(ctx, userID)

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
	}

	var rows []deviceKeyCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), selectAllDeviceKeysSQL, params, &rows)

	if err != nil {
		return err
	}

	for _, item := range rows {
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
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
	}

	var rows []deviceKeyCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), s.selectBatchDeviceKeysStmt, params, &rows)

	// rows, err := s.selectBatchDeviceKeysStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	// defer internal.CloseAndLogIfError(ctx, rows, "selectBatchDeviceKeysStmt: rows.close() failed")

	var result []api.DeviceMessage
	for _, item := range rows {
		dk := api.DeviceMessage{
			Type:       api.TypeDeviceKeyUpdate,
			DeviceKeys: &api.DeviceKeys{},
		}
		dk.Type = api.TypeDeviceKeyUpdate
		dk.UserID = item.DeviceKey.UserID
		// var keyJSON string
		var streamID int
		// var displayName sql.NullString
		// if err := rows.Scan(&dk.DeviceID, &keyJSON, &streamID, &displayName); err != nil {
		// 	return nil, err
		// }
		dk.DeviceID = item.DeviceKey.DeviceID
		dk.KeyJSON = item.DeviceKey.KeyJSON
		streamID = item.DeviceKey.StreamID
		displayName := item.DeviceKey.DisplayName
		dk.StreamID = streamID
		if len(displayName) > 0 {
			dk.DisplayName = displayName
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

		//     UNIQUE (user_id, device_id)
		docId := fmt.Sprintf("%s_%s", key.UserID, key.DeviceID)
		cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

		response, err := getDeviceKey(s, ctx, s.getPartitionKey(key.UserID), cosmosDocId)

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

	params := map[string]interface{}{
		"@x1": s.db.cosmosConfig.TenantName,
		"@x2": s.getCollectionName(),
		"@x3": userID,
	}

	// err = sqlutil.TxStmt(txn, s.selectMaxStreamForUserStmt).QueryRowContext(ctx, userID).Scan(&nullStream)
	var rows []deviceKeyCosmosNumber
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), selectMaxStreamForUserSQL, params, &rows)

	if err != nil {
		if err == cosmosdbutil.ErrNoRows {
			err = nil
		} else {
			return nullStream.Int32, err
		}
	}

	if len(rows) > 0 {
		nullStream.Int32 = int32(rows[0].Number)
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

	params := map[string]interface{}{
		"@x1": s.db.cosmosConfig.TenantName,
		"@x2": s.getCollectionName(),
		"@x3": userID,
		"@x4": iStreamIDs,
	}

	// query := strings.Replace(countStreamIDsForUserSQL, "($2)", sqlutil.QueryVariadicOffset(len(streamIDs), 1), 1)
	// // nullable if there are no results
	// var count sql.NullInt32
	// err := s.db.QueryRowContext(ctx, query, iStreamIDs...).Scan(&count)

	var rows []deviceKeyCosmosNumber
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), countStreamIDsForUserSQL, params, &rows)

	if err != nil {
		return 0, err
	}
	// if count.Valid {
	// 	return int(count.Int32), nil
	// }
	if rows[0].Number >= 0 {
		return int(rows[0].Number), nil
	}
	return 0, nil
}

func (s *deviceKeysStatements) InsertDeviceKeys(ctx context.Context, txn *sql.Tx, keys []api.DeviceMessage) error {

	// "INSERT INTO keyserver_device_keys (user_id, device_id, ts_added_secs, key_json, stream_id, display_name)" +
	// " VALUES ($1, $2, $3, $4, $5, $6)" +
	// " ON CONFLICT (user_id, device_id)" +
	// " DO UPDATE SET key_json = $4, stream_id = $5, display_name = $6"

	for _, key := range keys {
		//     UNIQUE (user_id, device_id)
		docId := fmt.Sprintf("%s_%s", key.UserID, key.DeviceID)
		cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

		dbData := &deviceKeyCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(key.UserID), cosmosDocId),
			DeviceKey:      mapFromDeviceKeyMessage(key),
		}

		err := insertDeviceKeyCore(s, ctx, *dbData)

		if err != nil {
			return err
		}
	}
	return nil
}
