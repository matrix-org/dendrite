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
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

// var oneTimeKeysSchema = `
// -- Stores one-time public keys for users
// CREATE TABLE IF NOT EXISTS keyserver_one_time_keys (
//     user_id TEXT NOT NULL,
// 	device_id TEXT NOT NULL,
// 	key_id TEXT NOT NULL,
// 	algorithm TEXT NOT NULL,
// 	ts_added_secs BIGINT NOT NULL,
// 	key_json TEXT NOT NULL,
// 	-- Clobber based on 4-uple of user/device/key/algorithm.
//     UNIQUE (user_id, device_id, key_id, algorithm)
// );
// `

type oneTimeKeyCosmos struct {
	UserID    string `json:"user_id"`
	DeviceID  string `json:"device_id"`
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	// Use the CosmosDB.Timestamp for this one
	// ts_added_secs    int64  `json:"ts_added_secs"`
	KeyJSON []byte `json:"key_json"`
}

type oneTimeKeyAlgoNumberCosmosData struct {
	Algorithm string `json:"algorithm"`
	Number    int    `json:"number"`
}

type oneTimeKeyCosmosData struct {
	cosmosdbapi.CosmosDocument
	OneTimeKey oneTimeKeyCosmos `json:"mx_keyserver_one_time_key"`
}

// const upsertKeysSQL = "" +
// 	"INSERT INTO keyserver_one_time_keys (user_id, device_id, key_id, algorithm, ts_added_secs, key_json)" +
// 	" VALUES ($1, $2, $3, $4, $5, $6)" +
// 	" ON CONFLICT (user_id, device_id, key_id, algorithm)" +
// 	" DO UPDATE SET key_json = $6"

//  "SELECT key_id, algorithm, key_json FROM keyserver_one_time_keys WHERE user_id=$1 AND device_id=$2"
const selectKeysSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_one_time_key.user_id = @x2 " +
	"and c.mx_keyserver_one_time_key.device_id = @x3 "

// "SELECT algorithm, COUNT(key_id) FROM keyserver_one_time_keys WHERE user_id=$1 AND device_id=$2 GROUP BY algorithm"
const selectKeysCountSQL = "" +
	"select c.mx_keyserver_one_time_key.algorithm, count(c.mx_keyserver_one_time_key.key_id) as number " +
	"from c where c._cn = @x1 " +
	"and c.mx_keyserver_one_time_key.user_id = @x2 " +
	"and c.mx_keyserver_one_time_key.device_id = @x3 " +
	"group by c.mx_keyserver_one_time_key.algorithm "

const deleteOneTimeKeySQL = "" +
	"DELETE FROM keyserver_one_time_keys WHERE user_id = $1 AND device_id = $2 AND algorithm = $3 AND key_id = $4"

// "SELECT key_id, key_json FROM keyserver_one_time_keys WHERE user_id = $1 AND device_id = $2 AND algorithm = $3 LIMIT 1"
const selectKeyByAlgorithmSQL = "" +
	"select top 1 * from c where c._cn = @x1 " +
	"and c.mx_keyserver_one_time_key.user_id = @x2 " +
	"and c.mx_keyserver_one_time_key.device_id = @x3 " +
	"and c.mx_keyserver_one_time_key.algorithm = @x4 "

type oneTimeKeysStatements struct {
	db *Database
	// upsertKeysStmt           *sql.Stmt
	selectKeysStmt           string
	selectKeysCountStmt      string
	selectKeyByAlgorithmStmt string
	// deleteOneTimeKeyStmt     *sql.Stmt
	tableName string
}

func (s *oneTimeKeysStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *oneTimeKeysStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getOneTimeKey(s *oneTimeKeysStatements, ctx context.Context, pk string, docId string) (*oneTimeKeyCosmosData, error) {
	response := oneTimeKeyCosmosData{}
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

func insertOneTimeKeyCore(s *oneTimeKeysStatements, ctx context.Context, dbData oneTimeKeyCosmosData) error {
	// "INSERT INTO keyserver_one_time_keys (user_id, device_id, key_id, algorithm, ts_added_secs, key_json)" +
	// " VALUES ($1, $2, $3, $4, $5, $6)" +
	// " ON CONFLICT (user_id, device_id, key_id, algorithm)" +
	// " DO UPDATE SET key_json = $6"
	existing, _ := getOneTimeKey(s, ctx, dbData.Pk, dbData.Id)
	if existing != nil {
		existing.SetUpdateTime()
		existing.OneTimeKey.KeyJSON = dbData.OneTimeKey.KeyJSON
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

func deleteOneTimeKeyCore(s *oneTimeKeysStatements, ctx context.Context, dbData oneTimeKeyCosmosData) error {
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

func NewCosmosDBOneTimeKeysTable(db *Database) (tables.OneTimeKeys, error) {
	s := &oneTimeKeysStatements{
		db: db,
	}
	s.selectKeysStmt = selectKeysSQL
	s.selectKeysCountStmt = selectKeysCountSQL
	s.selectKeyByAlgorithmStmt = selectKeyByAlgorithmSQL
	s.tableName = "one_time_keys"
	return s, nil
}

func (s *oneTimeKeysStatements) SelectOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error) {

	//  "SELECT key_id, algorithm, key_json FROM keyserver_one_time_keys WHERE user_id=$1 AND device_id=$2"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": deviceID,
	}

	var rows []oneTimeKeyCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectKeyByAlgorithmStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	wantSet := make(map[string]bool, len(keyIDsWithAlgorithms))
	for _, ka := range keyIDsWithAlgorithms {
		wantSet[ka] = true
	}

	result := make(map[string]json.RawMessage)
	for _, item := range rows {
		var keyID string
		var algorithm string
		keyID = item.OneTimeKey.KeyID
		algorithm = item.OneTimeKey.Algorithm

		keyIDWithAlgo := algorithm + ":" + keyID
		if wantSet[keyIDWithAlgo] {
			result[keyIDWithAlgo] = item.OneTimeKey.KeyJSON
		}
	}
	return result, nil
}

func (s *oneTimeKeysStatements) CountOneTimeKeys(ctx context.Context, userID, deviceID string) (*api.OneTimeKeysCount, error) {
	counts := &api.OneTimeKeysCount{
		DeviceID: deviceID,
		UserID:   userID,
		KeyCount: make(map[string]int),
	}
	// rows, err := s.selectKeysCountStmt.QueryContext(ctx, userID, deviceID)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": counts.UserID,
		"@x3": counts.DeviceID,
	}

	// "SELECT algorithm, COUNT(key_id) FROM keyserver_one_time_keys WHERE user_id=$1 AND device_id=$2 GROUP BY algorithm"
	var rows []oneTimeKeyAlgoNumberCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectKeysCountStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	for _, item := range rows {
		var algorithm string
		var count int
		algorithm = item.Algorithm
		count = item.Number
		counts.KeyCount[algorithm] = count
	}
	return counts, nil
}

func (s *oneTimeKeysStatements) InsertOneTimeKeys(
	ctx context.Context, txn *sql.Tx, keys api.OneTimeKeys,
) (*api.OneTimeKeysCount, error) {
	counts := &api.OneTimeKeysCount{
		DeviceID: keys.DeviceID,
		UserID:   keys.UserID,
		KeyCount: make(map[string]int),
	}

	for keyIDWithAlgo, keyJSON := range keys.KeyJSON {

		// "INSERT INTO keyserver_one_time_keys (user_id, device_id, key_id, algorithm, ts_added_secs, key_json)" +
		// " VALUES ($1, $2, $3, $4, $5, $6)" +
		// " ON CONFLICT (user_id, device_id, key_id, algorithm)" +
		// " DO UPDATE SET key_json = $6"

		algo, keyID := keys.Split(keyIDWithAlgo)

		//     UNIQUE (user_id, device_id, key_id, algorithm)
		docId := fmt.Sprintf("%s_%s_%s_%s", keys.UserID, keys.DeviceID, keyID, algo)
		cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

		data := oneTimeKeyCosmos{
			Algorithm: algo,
			DeviceID:  keys.DeviceID,
			KeyID:     keyID,
			KeyJSON:   keyJSON,
			UserID:    keys.UserID,
		}

		dbData := &oneTimeKeyCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			OneTimeKey:     data,
		}

		err := insertOneTimeKeyCore(s, ctx, *dbData)

		if err != nil {
			return nil, err
		}
	}
	// rows, err := sqlutil.TxStmt(txn, s.selectKeysCountStmt).QueryContext(ctx, keys.UserID, keys.DeviceID)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": keys.UserID,
		"@x3": keys.DeviceID,
	}

	// "SELECT algorithm, COUNT(key_id) FROM keyserver_one_time_keys WHERE user_id=$1 AND device_id=$2 GROUP BY algorithm"
	var rows []oneTimeKeyAlgoNumberCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectKeysCountStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	for _, item := range rows {
		var algorithm string
		var count int
		algorithm = item.Algorithm
		count = item.Number
		counts.KeyCount[algorithm] = count
	}

	return counts, nil
}

func (s *oneTimeKeysStatements) SelectAndDeleteOneTimeKey(
	ctx context.Context, txn *sql.Tx, userID, deviceID, algorithm string,
) (map[string]json.RawMessage, error) {
	var keyID string
	// var keyJSON string

	// "SELECT key_id, key_json FROM keyserver_one_time_keys WHERE user_id = $1 AND device_id = $2 AND algorithm = $3 LIMIT 1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": deviceID,
		"@x4": algorithm,
	}

	var rows []oneTimeKeyCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectKeyByAlgorithmStmt, params, &rows)

	if err != nil {
		if err == cosmosdbutil.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	keyID = rows[0].OneTimeKey.KeyID
	keyJSONBytes := rows[0].OneTimeKey.KeyJSON
	err = deleteOneTimeKeyCore(s, ctx, rows[0])
	if err != nil {
		return nil, err
	}
	if keyID == "" {
		return nil, nil
	}
	return map[string]json.RawMessage{
		algorithm + ":" + keyID: keyJSONBytes,
	}, err
}
