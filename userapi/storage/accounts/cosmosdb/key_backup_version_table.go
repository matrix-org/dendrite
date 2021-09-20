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
	"strconv"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
	"github.com/matrix-org/gomatrixserverlib"
)

// const keyBackupVersionTableSchema = `
// -- the metadata for each generation of encrypted e2e session backups
// CREATE TABLE IF NOT EXISTS account_e2e_room_keys_versions (
//     user_id TEXT NOT NULL,
// 	-- this means no 2 users will ever have the same version of e2e session backups which strictly
// 	-- isn't necessary, but this is easy to do rather than SELECT MAX(version)+1.
//     version INTEGER PRIMARY KEY AUTOINCREMENT,
//     algorithm TEXT NOT NULL,
//     auth_data TEXT NOT NULL,
// 	etag TEXT NOT NULL,
//     deleted INTEGER DEFAULT 0 NOT NULL
// );

// CREATE UNIQUE INDEX IF NOT EXISTS account_e2e_room_keys_versions_idx ON account_e2e_room_keys_versions(user_id, version);
// `

type KeyBackupVersionCosmosData struct {
	cosmosdbapi.CosmosDocument
	KeyBackupVersion KeyBackupVersionCosmos `json:"mx_userapi_account_e2e_room_keys_versions"`
}

type KeyBackupVersionCosmos struct {
	UserId    string `json:"user_id"`
	Version   int64  `json:"vesion"`
	Algorithm string `json:"algorithm"`
	AuthData  []byte `json:"auth_data"`
	Etag      string `json:"etag"`
	Deleted   int    `json:"deleted"`
}

type KeyBackupVersionCosmosNumber struct {
	Number int64 `json:"number"`
}

// const insertKeyBackupSQL = "" +
// 	"INSERT INTO account_e2e_room_keys_versions(user_id, algorithm, auth_data, etag) VALUES ($1, $2, $3, $4) RETURNING version"

// const updateKeyBackupAuthDataSQL = "" +
// 	"UPDATE account_e2e_room_keys_versions SET auth_data = $1 WHERE user_id = $2 AND version = $3"

// const updateKeyBackupETagSQL = "" +
// 	"UPDATE account_e2e_room_keys_versions SET etag = $1 WHERE user_id = $2 AND version = $3"

// const deleteKeyBackupSQL = "" +
// 	"UPDATE account_e2e_room_keys_versions SET deleted=1 WHERE user_id = $1 AND version = $2"

// const selectKeyBackupSQL = "" +
// 	"SELECT algorithm, auth_data, etag, deleted FROM account_e2e_room_keys_versions WHERE user_id = $1 AND version = $2"

// 	"SELECT MAX(version) FROM account_e2e_room_keys_versions WHERE user_id = $1"
const selectLatestVersionSQL = "" +
	"select max(c.mx_userapi_account_e2e_room_keys_versions.version) as number from c where c._sid = @x1 and c._cn = @x2 " +
	"and c.mx_userapi_account_e2e_room_keys_versions.user_id = @x3 "

type keyBackupVersionStatements struct {
	db *Database
	// insertKeyBackupStmt         *sql.Stmt
	// updateKeyBackupAuthDataStmt *sql.Stmt
	// deleteKeyBackupStmt         *sql.Stmt
	// selectKeyBackupStmt         *sql.Stmt
	selectLatestVersionStmt string
	// updateKeyBackupETagStmt     *sql.Stmt
	tableName  string
	serverName gomatrixserverlib.ServerName
}

func queryKeyBackupVersionNumber(s *keyBackupVersionStatements, ctx context.Context, qry string, params map[string]interface{}) ([]KeyBackupVersionCosmosNumber, error) {
	var response []KeyBackupVersionCosmosNumber

	var optionsQry = cosmosdbapi.GetQueryAllPartitionsDocumentsOptions()
	var query = cosmosdbapi.GetQuery(qry, params)
	var _, _ = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	//WHen there is no data these GroupBy queries return errors
	// if err != nil {
	// 	return nil, err
	// }

	if len(response) == 0 {
		return nil, cosmosdbutil.ErrNoRows
	}

	return response, nil
}

func getKeyBackupVersion(s *keyBackupVersionStatements, ctx context.Context, pk string, docId string) (*KeyBackupVersionCosmosData, error) {
	response := KeyBackupVersionCosmosData{}
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

func setKeyBackupVersion(s *keyBackupVersionStatements, ctx context.Context, keyBackup KeyBackupVersionCosmosData) (*KeyBackupVersionCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(keyBackup.Pk, keyBackup.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		keyBackup.Id,
		&keyBackup,
		optionsReplace)
	return &keyBackup, ex
}

func (s *keyBackupVersionStatements) prepare(db *Database, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	// s.insertKeyBackupStmt = insertKeyBackupSQL
	// s.updateKeyBackupAuthDataStmt = updateKeyBackupAuthDataSQL
	// s.deleteKeyBackupStmt = deleteKeyBackupSQL
	// s.selectKeyBackupStmt = selectKeyBackupSQL
	s.selectLatestVersionStmt = selectLatestVersionSQL
	// s.updateKeyBackupETagStmt = updateKeyBackupETagSQL
	s.tableName = "account_e2e_room_keys_versions"
	s.serverName = server
	return
}

func (s *keyBackupVersionStatements) insertKeyBackup(
	ctx context.Context, userID, algorithm string, authData json.RawMessage, etag string,
) (version string, err error) {
	// "INSERT INTO account_e2e_room_keys_versions(user_id, algorithm, auth_data, etag) VALUES ($1, $2, $3, $4) RETURNING version"
	var versionInt int64
	// 	-- this means no 2 users will ever have the same version of e2e session backups which strictly
	// 	-- isn't necessary, but this is easy to do rather than SELECT MAX(version)+1.
	//     version INTEGER PRIMARY KEY AUTOINCREMENT,
	versionInt, seqErr := GetNextKeyBackupVersionID(s, ctx)
	if seqErr != nil {
		return "", seqErr
	}
	// err = txn.Stmt(s.insertKeyBackupStmt).QueryRowContext(ctx, userID, algorithm, string(authData), etag).Scan(&versionInt)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS account_e2e_room_keys_versions_idx ON account_e2e_room_keys_versions(user_id, version);
	docId := fmt.Sprintf("%s_%d", userID, versionInt)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)

	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	data := KeyBackupVersionCosmos{
		UserId:    userID,
		Version:   versionInt,
		Algorithm: algorithm,
		AuthData:  authData,
		Etag:      etag,
		Deleted:   0,
	}

	dbData := &KeyBackupVersionCosmosData{
		CosmosDocument:   cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
		KeyBackupVersion: data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return strconv.FormatInt(versionInt, 10), err
}

func (s *keyBackupVersionStatements) updateKeyBackupAuthData(
	ctx context.Context, userID, version string, authData json.RawMessage,
) error {
	// 	"UPDATE account_e2e_room_keys_versions SET auth_data = $1 WHERE user_id = $2 AND version = $3"
	versionInt, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid version")
	}
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS account_e2e_room_keys_versions_idx ON account_e2e_room_keys_versions(user_id, version);
	docId := fmt.Sprintf("%s_%d", userID, versionInt)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	res, err := getKeyBackupVersion(s, ctx, pk, cosmosDocId)

	if err != nil {
		return err
	}

	if res == nil {
		return err
	}

	// _, err = txn.Stmt(s.updateKeyBackupAuthDataStmt).ExecContext(ctx, string(authData), userID, versionInt)
	res.KeyBackupVersion.AuthData = authData

	_, err = setKeyBackupVersion(s, ctx, *res)

	return err
}

func (s *keyBackupVersionStatements) updateKeyBackupETag(
	ctx context.Context, userID, version, etag string,
) error {
	// "UPDATE account_e2e_room_keys_versions SET etag = $1 WHERE user_id = $2 AND version = $3"
	versionInt, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid version")
	}
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS account_e2e_room_keys_versions_idx ON account_e2e_room_keys_versions(user_id, version);
	docId := fmt.Sprintf("%s_%d", userID, versionInt)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	res, err := getKeyBackupVersion(s, ctx, pk, cosmosDocId)

	if err != nil {
		return err
	}

	if res == nil {
		return err
	}

	// _, err = txn.Stmt(s.updateKeyBackupETagStmt).ExecContext(ctx, etag, userID, versionInt)
	res.KeyBackupVersion.Etag = etag

	_, err = setKeyBackupVersion(s, ctx, *res)

	return err
}

func (s *keyBackupVersionStatements) deleteKeyBackup(
	ctx context.Context, userID, version string,
) (bool, error) {
	// "UPDATE account_e2e_room_keys_versions SET deleted=1 WHERE user_id = $1 AND version = $2"
	versionInt, err := strconv.ParseInt(version, 10, 64)
	if err != nil {
		return false, fmt.Errorf("invalid version")
	}
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS account_e2e_room_keys_versions_idx ON account_e2e_room_keys_versions(user_id, version);
	docId := fmt.Sprintf("%s_%d", userID, versionInt)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	res, err := getKeyBackupVersion(s, ctx, pk, cosmosDocId)

	if err != nil {
		return false, err
	}

	if res == nil {
		return false, err
	}

	// result, err := txn.Stmt(s.deleteKeyBackupStmt).ExecContext(ctx, userID, versionInt)
	res.KeyBackupVersion.Deleted = 1

	_, err = setKeyBackupVersion(s, ctx, *res)

	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *keyBackupVersionStatements) selectKeyBackup(
	ctx context.Context, userID, version string,
) (versionResult, algorithm string, authData json.RawMessage, etag string, deleted bool, err error) {
	// "SELECT algorithm, auth_data, etag, deleted FROM account_e2e_room_keys_versions WHERE user_id = $1 AND version = $2"
	var versionInt int64
	if version == "" {
		// var v *int64 // allows nulls
		var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
		params := map[string]interface{}{
			"@x1": s.db.cosmosConfig.TenantName,
			"@x2": dbCollectionName,
			"@x3": userID,
		}

		// err = sqlutil.TxStmt(txn, s.selectMaxStreamForUserStmt).QueryRowContext(ctx, userID).Scan(&nullStream)
		response, err1 := queryKeyBackupVersionNumber(s, ctx, s.selectLatestVersionStmt, params)

		if err1 != nil {
			if err == cosmosdbutil.ErrNoRows {
				err = nil
			}
		}
		// if err = txn.Stmt(s.selectLatestVersionStmt).QueryRowContext(ctx, userID).Scan(&v); err != nil {
		// 	return
		// }
		if response == nil || len(response) == 0 {
			err = cosmosdbutil.ErrNoRows
			versionInt = 0
			return
		}
		versionInt = response[0].Number
	} else {
		if versionInt, err = strconv.ParseInt(version, 10, 64); err != nil {
			return
		}
	}
	versionResult = strconv.FormatInt(versionInt, 10)
	if err != nil {
		return
	}
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS account_e2e_room_keys_versions_idx ON account_e2e_room_keys_versions(user_id, version);
	docId := fmt.Sprintf("%s_%d", userID, versionInt)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	res, err := getKeyBackupVersion(s, ctx, pk, cosmosDocId)

	if err != nil {
		return
	}

	if res == nil {
		return
	}

	// var deletedInt int
	// var authDataStr string
	// err = txn.Stmt(s.selectKeyBackupStmt).QueryRowContext(ctx, userID, versionInt).Scan(&algorithm, &authDataStr, &etag, &deletedInt)
	deleted = res.KeyBackupVersion.Deleted == 1
	authData = res.KeyBackupVersion.AuthData
	return
}
