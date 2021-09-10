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
	"database/sql"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// var crossSigningKeysSchema = `
// CREATE TABLE IF NOT EXISTS keyserver_cross_signing_keys (
//     user_id TEXT NOT NULL,
// 	key_type INTEGER NOT NULL,
// 	key_data TEXT NOT NULL,
// 	PRIMARY KEY (user_id, key_type)
// );
// `

type CrossSigningKeysCosmos struct {
	UserID  string `json:"user_id"`
	KeyType int64  `json:"key_type"`
	KeyData []byte `json:"key_data"`
}

type CrossSigningKeysCosmosData struct {
	Id               string                 `json:"id"`
	Pk               string                 `json:"_pk"`
	Tn               string                 `json:"_sid"`
	Cn               string                 `json:"_cn"`
	ETag             string                 `json:"_etag"`
	Timestamp        int64                  `json:"_ts"`
	CrossSigningKeys CrossSigningKeysCosmos `json:"mx_keyserver_cross_signing_keys"`
}

// "SELECT key_type, key_data FROM keyserver_cross_signing_keys" +
// " WHERE user_id = $1"
const selectCrossSigningKeysForUserSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_cross_signing_keys.user_id = @x2 "

// const upsertCrossSigningKeysForUserSQL = "" +
// 	"INSERT OR REPLACE INTO keyserver_cross_signing_keys (user_id, key_type, key_data)" +
// 	" VALUES($1, $2, $3)"

type crossSigningKeysStatements struct {
	db                                *Database
	selectCrossSigningKeysForUserStmt string
	// upsertCrossSigningKeysForUserStmt *sql.Stmt
	tableName string
}

func queryCrossSigningKeys(s *crossSigningKeysStatements, ctx context.Context, qry string, params map[string]interface{}) ([]CrossSigningKeysCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []CrossSigningKeysCosmosData

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

func NewSqliteCrossSigningKeysTable(db *Database) (tables.CrossSigningKeys, error) {
	s := &crossSigningKeysStatements{
		db: db,
	}
	s.selectCrossSigningKeysForUserStmt = selectCrossSigningKeysForUserSQL
	// s.upsertCrossSigningKeysForUserStmt = upsertCrossSigningKeysForUserSQL
	s.tableName = "cross_signing_keys"
	return s, nil
}

func (s *crossSigningKeysStatements) SelectCrossSigningKeysForUser(
	ctx context.Context, txn *sql.Tx, userID string,
) (r types.CrossSigningKeyMap, err error) {
	// "SELECT key_type, key_data FROM keyserver_cross_signing_keys" +
	// " WHERE user_id = $1"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
	}
	rows, err := queryCrossSigningKeys(s, ctx, s.selectCrossSigningKeysForUserStmt, params)
	// rows, err := sqlutil.TxStmt(txn, s.selectCrossSigningKeysForUserStmt).QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	// defer internal.CloseAndLogIfError(ctx, rows, "selectCrossSigningKeysForUserStmt: rows.close() failed")
	r = types.CrossSigningKeyMap{}
	// for rows.Next() {
	for _, item := range rows {
		var keyTypeInt int16
		var keyData gomatrixserverlib.Base64Bytes
		// if err := rows.Scan(&keyTypeInt, &keyData); err != nil {
		// 	return nil, err
		// }
		keyData = item.CrossSigningKeys.KeyData
		keyTypeInt = int16(item.CrossSigningKeys.KeyType)
		keyType, ok := types.KeyTypeIntToPurpose[keyTypeInt]
		if !ok {
			return nil, fmt.Errorf("unknown key purpose int %d", keyTypeInt)
		}
		r[keyType] = keyData
	}
	return
}

func (s *crossSigningKeysStatements) UpsertCrossSigningKeysForUser(
	ctx context.Context, txn *sql.Tx, userID string, keyType gomatrixserverlib.CrossSigningKeyPurpose, keyData gomatrixserverlib.Base64Bytes,
) error {
	// "INSERT OR REPLACE INTO keyserver_cross_signing_keys (user_id, key_type, key_data)" +
	// " VALUES($1, $2, $3)"
	keyTypeInt, ok := types.KeyTypePurposeToInt[keyType]
	if !ok {
		return fmt.Errorf("unknown key purpose %q", keyType)
	}
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	// 	PRIMARY KEY (user_id, key_type)
	docId := fmt.Sprintf("%s_%s", userID, keyType)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)

	data := CrossSigningKeysCosmos{
		UserID:  userID,
		KeyType: int64(keyTypeInt),
		KeyData: keyData,
	}

	dbData := CrossSigningKeysCosmosData{
		Id:               cosmosDocId,
		Tn:               s.db.cosmosConfig.TenantName,
		Cn:               dbCollectionName,
		Pk:               pk,
		Timestamp:        time.Now().Unix(),
		CrossSigningKeys: data,
	}

	// if _, err := sqlutil.TxStmt(txn, s.upsertCrossSigningKeysForUserStmt).ExecContext(ctx, userID, keyTypeInt, keyData); err != nil {
	// 	return fmt.Errorf("s.upsertCrossSigningKeysForUserStmt: %w", err)
	// }
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	return err
}
