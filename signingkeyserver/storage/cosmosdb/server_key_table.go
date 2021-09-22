// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

// const serverKeysSchema = `
// -- A cache of signing keys downloaded from remote servers.
// CREATE TABLE IF NOT EXISTS keydb_server_keys (
// 	-- The name of the matrix server the key is for.
// 	server_name TEXT NOT NULL,
// 	-- The ID of the server key.
// 	server_key_id TEXT NOT NULL,
// 	-- Combined server name and key ID separated by the ASCII unit separator
// 	-- to make it easier to run bulk queries.
// 	server_name_and_key_id TEXT NOT NULL,
// 	-- When the key is valid until as a millisecond timestamp.
// 	-- 0 if this is an expired key (in which case expired_ts will be non-zero)
// 	valid_until_ts BIGINT NOT NULL,
// 	-- When the key expired as a millisecond timestamp.
// 	-- 0 if this is an active key (in which case valid_until_ts will be non-zero)
// 	expired_ts BIGINT NOT NULL,
// 	-- The base64-encoded public key.
// 	server_key TEXT NOT NULL,
// 	UNIQUE (server_name, server_key_id)
// );

// CREATE INDEX IF NOT EXISTS keydb_server_name_and_key_id ON keydb_server_keys (server_name_and_key_id);
// `

type serverKeyCosmos struct {
	ServerName          string `json:"server_name"`
	ServerKeyID         string `json:"server_key_id"`
	ServerNameAndKeyID  string `json:"server_name_and_key_id"`
	ValidUntilTimestamp int64  `json:"valid_until_ts"`
	ExpiredTimestamp    int64  `json:"expired_ts"`
	ServerKey           string `json:"server_key"`
}

type serverKeyCosmosData struct {
	cosmosdbapi.CosmosDocument
	ServerKey serverKeyCosmos `json:"mx_keydb_server_key"`
}

// "SELECT server_name, server_key_id, valid_until_ts, expired_ts, " +
// "   server_key FROM keydb_server_keys" +
// " WHERE server_name_and_key_id IN ($1)"
const bulkSelectServerKeysSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_keydb_server_key.server_name_and_key_id) "

// const upsertServerKeysSQL = "" +
// 	"INSERT INTO keydb_server_keys (server_name, server_key_id," +
// 	" server_name_and_key_id, valid_until_ts, expired_ts, server_key)" +
// 	" VALUES ($1, $2, $3, $4, $5, $6)" +
// 	" ON CONFLICT (server_name, server_key_id)" +
// 	" DO UPDATE SET valid_until_ts = $4, expired_ts = $5, server_key = $6"

type serverKeyStatements struct {
	db                       *Database
	writer                   cosmosdbutil.Writer
	bulkSelectServerKeysStmt *sql.Stmt
	// upsertServerKeysStmt     *sql.Stmt
	tableName string
}

func (s *serverKeyStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *serverKeyStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getServerKey(s *serverKeyStatements, ctx context.Context, pk string, docId string) (*serverKeyCosmosData, error) {
	response := serverKeyCosmosData{}
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

func (s *serverKeyStatements) prepare(db *Database, writer sqlutil.Writer) (err error) {
	s.db = db
	s.writer = writer
	s.tableName = "server_keys"
	return
}

func (s *serverKeyStatements) bulkSelectServerKeys(
	ctx context.Context,
	requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp,
) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error) {
	nameAndKeyIDs := make([]string, 0, len(requests))
	for request := range requests {
		nameAndKeyIDs = append(nameAndKeyIDs, nameAndKeyID(request))
	}
	results := make(map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, len(requests))
	// iKeyIDs := make([]interface{}, len(nameAndKeyIDs))
	// for i, v := range nameAndKeyIDs {
	// 	iKeyIDs[i] = v
	// }

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": nameAndKeyIDs,
	}

	// "SELECT server_name, server_key_id, valid_until_ts, expired_ts, " +
	// "   server_key FROM keydb_server_keys" +
	// " WHERE server_name_and_key_id IN ($1)"

	// err := sqlutil.RunLimitedVariablesQuery(
	// 	ctx, bulkSelectServerKeysSQL, s.db, iKeyIDs, sqlutil.SQLite3MaxVariables,
	// 	func(rows *sql.Rows) error {
	var rows []serverKeyCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), bulkSelectServerKeysSQL, params, &rows)

	if err != nil {
		return nil, err
	}

	for _, item := range rows {
		var serverName string
		var keyID string
		var key string
		var validUntilTS int64
		var expiredTS int64
		// if err := rows.Scan(&serverName, &keyID, &validUntilTS, &expiredTS, &key); err != nil {
		// 	return fmt.Errorf("bulkSelectServerKeys: %v", err)
		// }
		serverName = item.ServerKey.ServerName
		keyID = item.ServerKey.ServerKeyID
		validUntilTS = item.ServerKey.ValidUntilTimestamp
		expiredTS = item.ServerKey.ExpiredTimestamp
		key = item.ServerKey.ServerKey
		r := gomatrixserverlib.PublicKeyLookupRequest{
			ServerName: gomatrixserverlib.ServerName(serverName),
			KeyID:      gomatrixserverlib.KeyID(keyID),
		}
		vk := gomatrixserverlib.VerifyKey{}
		err := vk.Key.Decode(key)
		if err != nil {
			return nil, fmt.Errorf("bulkSelectServerKeys: %v", err)
		}
		results[r] = gomatrixserverlib.PublicKeyLookupResult{
			VerifyKey:    vk,
			ValidUntilTS: gomatrixserverlib.Timestamp(validUntilTS),
			ExpiredTS:    gomatrixserverlib.Timestamp(expiredTS),
		}
	}

	return results, err
}

func (s *serverKeyStatements) upsertServerKeys(
	ctx context.Context,
	request gomatrixserverlib.PublicKeyLookupRequest,
	key gomatrixserverlib.PublicKeyLookupResult,
) error {

	// "INSERT INTO keydb_server_keys (server_name, server_key_id," +
	// " server_name_and_key_id, valid_until_ts, expired_ts, server_key)" +
	// " VALUES ($1, $2, $3, $4, $5, $6)" +
	// " ON CONFLICT (server_name, server_key_id)" +
	// " DO UPDATE SET valid_until_ts = $4, expired_ts = $5, server_key = $6"

	// stmt := sqlutil.TxStmt(txn, s.upsertServerKeysStmt)

	// 	UNIQUE (server_name, server_key_id)
	docId := fmt.Sprintf("%s_%s", string(request.ServerName), string(request.KeyID))
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getServerKey(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.ServerKey.ValidUntilTimestamp = int64(key.ValidUntilTS)
		dbData.ServerKey.ExpiredTimestamp = int64(key.ExpiredTS)
		dbData.ServerKey.ServerKey = key.Key.Encode()
	} else {
		data := serverKeyCosmos{
			ServerName:          string(request.ServerName),
			ServerKeyID:         string(request.KeyID),
			ServerNameAndKeyID:  nameAndKeyID(request),
			ValidUntilTimestamp: int64(key.ValidUntilTS),
			ExpiredTimestamp:    int64(key.ExpiredTS),
			ServerKey:           key.Key.Encode(),
		}

		dbData = &serverKeyCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			ServerKey:      data,
		}
	}
	// _, err := stmt.ExecContext(
	// 	ctx,
	// 	string(request.ServerName),
	// 	string(request.KeyID),
	// 	nameAndKeyID(request),
	// 	key.ValidUntilTS,
	// 	key.ExpiredTS,
	// 	key.Key.Encode(),
	// )

	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func nameAndKeyID(request gomatrixserverlib.PublicKeyLookupRequest) string {
	return string(request.ServerName) + "\x1F" + string(request.KeyID)
}
