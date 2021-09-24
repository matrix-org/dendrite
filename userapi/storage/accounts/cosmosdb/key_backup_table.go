// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// const keyBackupTableSchema = `
// CREATE TABLE IF NOT EXISTS account_e2e_room_keys (
//     user_id TEXT NOT NULL,
//     room_id TEXT NOT NULL,
//     session_id TEXT NOT NULL,

//     version TEXT NOT NULL,
//     first_message_index INTEGER NOT NULL,
//     forwarded_count INTEGER NOT NULL,
//     is_verified BOOLEAN NOT NULL,
//     session_data TEXT NOT NULL
// );
// CREATE UNIQUE INDEX IF NOT EXISTS e2e_room_keys_idx ON account_e2e_room_keys(user_id, room_id, session_id, version);
// CREATE INDEX IF NOT EXISTS e2e_room_keys_versions_idx ON account_e2e_room_keys(user_id, version);
// `

type keyBackupCosmos struct {
	UserId            string `json:"user_id"`
	RoomId            string `json:"room_id"`
	SessionId         string `json:"session_id"`
	Version           string `json:"vesion"`
	FirstMessageIndex int    `json:"first_message_index"`
	ForwardedCount    int    `json:"forwarded_count"`
	IsVerified        bool   `json:"is_verified"`
	SessionData       []byte `json:"session_data"`
}

type keyBackupCosmosData struct {
	cosmosdbapi.CosmosDocument
	KeyBackup keyBackupCosmos `json:"mx_userapi_account_e2e_room_keys"`
}

type keyBackupCosmosNumber struct {
	Number int64 `json:"number"`
}

// const insertBackupKeySQL = "" +
// 	"INSERT INTO account_e2e_room_keys(user_id, room_id, session_id, version, first_message_index, forwarded_count, is_verified, session_data) " +
// 	"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"

// const updateBackupKeySQL = "" +
// 	"UPDATE account_e2e_room_keys SET first_message_index=$1, forwarded_count=$2, is_verified=$3, session_data=$4 " +
// 	"WHERE user_id=$5 AND room_id=$6 AND session_id=$7 AND version=$8"

// "SELECT COUNT(*) FROM account_e2e_room_keys WHERE user_id = $1 AND version = $2"
const countKeysSQL = "" +
	"select count(c._ts) as number from c where c._cn = @x1 " +
	"and c.mx_userapi_account_e2e_room_keys.user_id = @x2 " +
	"and c.mx_userapi_account_e2e_room_keys.version = @x3 "

// "SELECT room_id, session_id, first_message_index, forwarded_count, is_verified, session_data FROM account_e2e_room_keys " +
// "WHERE user_id = $1 AND version = $2"
const selectKeysSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_userapi_account_e2e_room_keys.user_id = @x2 " +
	"and c.mx_userapi_account_e2e_room_keys.version = @x3 "

// "SELECT room_id, session_id, first_message_index, forwarded_count, is_verified, session_data FROM account_e2e_room_keys " +
// "WHERE user_id = $1 AND version = $2 AND room_id = $3"
const selectKeysByRoomIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_userapi_account_e2e_room_keys.user_id = @x2 " +
	"and c.mx_userapi_account_e2e_room_keys.version = @x3 " +
	"and c.mx_userapi_account_e2e_room_keys.room_id = @x4 "

// "SELECT room_id, session_id, first_message_index, forwarded_count, is_verified, session_data FROM account_e2e_room_keys " +
// "WHERE user_id = $1 AND version = $2 AND room_id = $3 AND session_id = $4"
const selectKeysByRoomIDAndSessionIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_userapi_account_e2e_room_keys.user_id = @x2 " +
	"and c.mx_userapi_account_e2e_room_keys.version = @x3 " +
	"and c.mx_userapi_account_e2e_room_keys.room_id = @x4 " +
	"and c.mx_userapi_account_e2e_room_keys.session_id = @x5 "

type keyBackupStatements struct {
	db *Database
	// insertBackupKeyStmt                *sql.Stmt
	// updateBackupKeyStmt                *sql.Stmt
	countKeysStmt                      string
	selectKeysStmt                     string
	selectKeysByRoomIDStmt             string
	selectKeysByRoomIDAndSessionIDStmt string
	tableName                          string
	serverName                         gomatrixserverlib.ServerName
}

func (s *keyBackupStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *keyBackupStatements) getPartitionKey(userId string) string {
	uniqueId := userId
	return cosmosdbapi.GetPartitionKeyByUniqueId(s.db.cosmosConfig.TenantName, s.getCollectionName(), uniqueId)
}

func getKeyBackup(s *keyBackupStatements, ctx context.Context, pk string, docId string) (*keyBackupCosmosData, error) {
	response := keyBackupCosmosData{}
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

func (s *keyBackupStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	// s.insertBackupKeyStmt = insertBackupKeySQL
	// s.updateBackupKeyStmt = updateBackupKeySQL
	s.countKeysStmt = countKeysSQL
	s.selectKeysStmt = selectKeysSQL
	s.selectKeysByRoomIDStmt = selectKeysByRoomIDSQL
	s.selectKeysByRoomIDAndSessionIDStmt = selectKeysByRoomIDAndSessionIDSQL
	s.tableName = "account_e2e_room_keys"
	s.serverName = server
	return
}

func (s keyBackupStatements) countKeys(
	ctx context.Context, userID, version string,
) (count int64, err error) {
	// "SELECT COUNT(*) FROM account_e2e_room_keys WHERE user_id = $1 AND version = $2"
	// err = txn.Stmt(s.countKeysStmt).QueryRowContext(ctx, userID, version).Scan(&count)

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": version,
	}
	var rows []keyBackupCosmosNumber
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), s.countKeysStmt, params, &rows)

	if err != nil {
		return -1, err
	}

	if len(rows) == 0 {
		return -1, nil
	}
	// err := stmt.QueryRowContext(ctx, jsonNID).Scan(&count)
	count = rows[0].Number
	return
}

func (s *keyBackupStatements) insertBackupKey(
	ctx context.Context, userID, version string, key api.InternalKeyBackupSession,
) (err error) {
	// 	"INSERT INTO account_e2e_room_keys(user_id, room_id, session_id, version, first_message_index, forwarded_count, is_verified, session_data) " +
	// 	"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"
	// _, err = txn.Stmt(s.insertBackupKeyStmt).ExecContext(
	// 	ctx, userID, key.RoomID, key.SessionID, version, key.FirstMessageIndex, key.ForwardedCount, key.IsVerified, string(key.SessionData),
	// )
	// CREATE UNIQUE INDEX IF NOT EXISTS e2e_room_keys_idx ON account_e2e_room_keys(user_id, room_id, session_id, version);
	docId := fmt.Sprintf("%s_%s_%s_%s", userID, key.RoomID, key.SessionID, version)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := keyBackupCosmos{
		UserId:            userID,
		RoomId:            key.RoomID,
		SessionId:         key.SessionID,
		Version:           version,
		FirstMessageIndex: key.FirstMessageIndex,
		ForwardedCount:    key.ForwardedCount,
		IsVerified:        key.IsVerified,
		SessionData:       key.SessionData,
	}

	dbData := &keyBackupCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(userID), cosmosDocId),
		KeyBackup:      data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return
}

func (s *keyBackupStatements) updateBackupKey(
	ctx context.Context, userID, version string, key api.InternalKeyBackupSession,
) (err error) {
	// 	"UPDATE account_e2e_room_keys SET first_message_index=$1, forwarded_count=$2, is_verified=$3, session_data=$4 " +
	// 	"WHERE user_id=$5 AND room_id=$6 AND session_id=$7 AND version=$8"
	// _, err = txn.Stmt(s.updateBackupKeyStmt).ExecContext(
	// 	ctx, key.FirstMessageIndex, key.ForwardedCount, key.IsVerified, string(key.SessionData), userID, key.RoomID, key.SessionID, version,
	// )

	// CREATE UNIQUE INDEX IF NOT EXISTS e2e_room_keys_idx ON account_e2e_room_keys(user_id, room_id, session_id, version);
	docId := fmt.Sprintf("%s_%s_%s_%s", userID, key.RoomID, key.SessionID, version)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	item, err := getKeyBackup(s, ctx, s.getPartitionKey(userID), cosmosDocId)

	if err != nil {
		return
	}

	if item == nil {
		return
	}

	// 	ctx, key.FirstMessageIndex, key.ForwardedCount, key.IsVerified, string(key.SessionData), userID, key.RoomID, key.SessionID, version,
	item.SetUpdateTime()
	item.KeyBackup.FirstMessageIndex = key.FirstMessageIndex
	item.KeyBackup.ForwardedCount = key.ForwardedCount
	item.KeyBackup.IsVerified = key.IsVerified
	item.KeyBackup.SessionData = key.SessionData

	_, err = cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)

	return
}

func (s *keyBackupStatements) selectKeys(
	ctx context.Context, userID, version string,
) (map[string]map[string]api.KeyBackupSession, error) {
	// "SELECT room_id, session_id, first_message_index, forwarded_count, is_verified, session_data FROM account_e2e_room_keys " +
	// "WHERE user_id = $1 AND version = $2"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": version,
	}
	var rows []keyBackupCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), s.selectKeysStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// rows, err := txn.Stmt(s.selectKeysStmt).QueryContext(ctx, userID, version)
	return unpackKeys(ctx, &rows)
}

func (s *keyBackupStatements) selectKeysByRoomID(
	ctx context.Context, userID, version, roomID string,
) (map[string]map[string]api.KeyBackupSession, error) {
	// "SELECT room_id, session_id, first_message_index, forwarded_count, is_verified, session_data FROM account_e2e_room_keys " +
	// "WHERE user_id = $1 AND version = $2 AND room_id = $3"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": version,
		"@x4": roomID,
	}
	var rows []keyBackupCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), s.selectKeysByRoomIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil
	}
	// rows, err := txn.Stmt(s.selectKeysByRoomIDStmt).QueryContext(ctx, userID, version, roomID)
	if err != nil {
		return nil, err
	}
	return unpackKeys(ctx, &rows)
}

func (s *keyBackupStatements) selectKeysByRoomIDAndSessionID(
	ctx context.Context, userID, version, roomID, sessionID string,
) (map[string]map[string]api.KeyBackupSession, error) {
	// "SELECT room_id, session_id, first_message_index, forwarded_count, is_verified, session_data FROM account_e2e_room_keys " +
	// "WHERE user_id = $1 AND version = $2 AND room_id = $3 AND session_id = $4"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": version,
		"@x4": roomID,
		"@x5": sessionID,
	}
	var rows []keyBackupCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(userID), s.selectKeysByRoomIDAndSessionIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return nil, nil
	}
	// rows, err := txn.Stmt(s.selectKeysByRoomIDAndSessionIDStmt).QueryContext(ctx, userID, version, roomID, sessionID)
	if err != nil {
		return nil, err
	}
	return unpackKeys(ctx, &rows)
}

func unpackKeys(ctx context.Context, rows *[]keyBackupCosmosData) (map[string]map[string]api.KeyBackupSession, error) {
	result := make(map[string]map[string]api.KeyBackupSession)
	for _, item := range *rows {
		var key api.InternalKeyBackupSession
		// room_id, session_id, first_message_index, forwarded_count, is_verified, session_data
		var sessionDataStr string
		// if err := rows.Scan(&key.RoomID, &key.SessionID, &key.FirstMessageIndex, &key.ForwardedCount, &key.IsVerified, &sessionDataStr); err != nil {
		// 	return nil, err
		// }
		key.RoomID = item.KeyBackup.RoomId
		key.SessionID = item.KeyBackup.SessionId
		key.FirstMessageIndex = item.KeyBackup.FirstMessageIndex
		key.ForwardedCount = item.KeyBackup.ForwardedCount
		key.SessionData = json.RawMessage(sessionDataStr)
		roomData := result[key.RoomID]
		if roomData == nil {
			roomData = make(map[string]api.KeyBackupSession)
		}
		roomData[key.SessionID] = key.KeyBackupSession
		result[key.RoomID] = roomData
	}
	return result, nil
}
