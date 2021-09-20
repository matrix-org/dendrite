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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// var crossSigningSigsSchema = `
// CREATE TABLE IF NOT EXISTS keyserver_cross_signing_sigs (
//     origin_user_id TEXT NOT NULL,
// 	origin_key_id TEXT NOT NULL,
// 	target_user_id TEXT NOT NULL,
// 	target_key_id TEXT NOT NULL,
// 	signature TEXT NOT NULL,
// 	PRIMARY KEY (origin_user_id, target_user_id, target_key_id)
// );
// `

type CrossSigningSigsCosmos struct {
	OriginUserId string `json:"origin_user_id"`
	OriginKeyId  string `json:"origin_key_id"`
	TargetUserId string `json:"target_user_id"`
	TargetKeyId  string `json:"target_key_id"`
	Signature    []byte `json:"signature"`
}

type CrossSigningSigsCosmosData struct {
	cosmosdbapi.CosmosDocument
	CrossSigningSigs CrossSigningSigsCosmos `json:"mx_keyserver_cross_signing_sigs"`
}

// "SELECT origin_user_id, origin_key_id, signature FROM keyserver_cross_signing_sigs" +
// " WHERE target_user_id = $1 AND target_key_id = $2"
const selectCrossSigningSigsForTargetSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_cross_signing_sigs.target_user_id = @x2 " +
	"and c.mx_keyserver_cross_signing_sigs.target_key_id = @x3 "

// const upsertCrossSigningSigsForTargetSQL = "" +
// 	"INSERT OR REPLACE INTO keyserver_cross_signing_sigs (origin_user_id, origin_key_id, target_user_id, target_key_id, signature)" +
// 	" VALUES($1, $2, $3, $4, $5)"

// 	"DELETE FROM keyserver_cross_signing_sigs WHERE target_user_id=$1 AND target_key_id=$2"
const deleteCrossSigningSigsForTargetSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_cross_signing_sigs.target_user_id = @x2 " +
	"and c.mx_keyserver_cross_signing_sigs.target_key_id = @x3 "

type crossSigningSigsStatements struct {
	db                                  *Database
	selectCrossSigningSigsForTargetStmt string
	// upsertCrossSigningSigsForTargetStmt *sql.Stmt
	deleteCrossSigningSigsForTargetStmt string
	tableName                           string
}

func getCrossSigningSigs(s *crossSigningSigsStatements, ctx context.Context, pk string, docId string) (*CrossSigningSigsCosmosData, error) {
	response := CrossSigningSigsCosmosData{}
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

func queryCrossSigningSigs(s *crossSigningSigsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]CrossSigningSigsCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []CrossSigningSigsCosmosData

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

func deleteCrossSigningSigs(s *crossSigningSigsStatements, ctx context.Context, dbData CrossSigningSigsCosmosData) error {
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

func NewSqliteCrossSigningSigsTable(db *Database) (tables.CrossSigningSigs, error) {
	s := &crossSigningSigsStatements{
		db: db,
	}
	// _, err := db.Exec(crossSigningSigsSchema)
	// if err != nil {
	// 	return nil, err
	// }
	s.selectCrossSigningSigsForTargetStmt = selectCrossSigningSigsForTargetSQL
	// s.upsertCrossSigningSigsForTargetStmt = upsertCrossSigningSigsForTargetSQL
	s.deleteCrossSigningSigsForTargetStmt = deleteCrossSigningSigsForTargetSQL
	s.tableName = "cross_signing_sigs"
	return s, nil
}

func (s *crossSigningSigsStatements) SelectCrossSigningSigsForTarget(
	ctx context.Context, txn *sql.Tx, targetUserID string, targetKeyID gomatrixserverlib.KeyID,
) (r types.CrossSigningSigMap, err error) {
	// "SELECT origin_user_id, origin_key_id, signature FROM keyserver_cross_signing_sigs" +
	// " WHERE target_user_id = $1 AND target_key_id = $2"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": targetUserID,
		"@x3": targetKeyID,
	}
	rows, err := queryCrossSigningSigs(s, ctx, s.selectCrossSigningSigsForTargetStmt, params)
	// rows, err := sqlutil.TxStmt(txn, s.selectCrossSigningSigsForTargetStmt).QueryContext(ctx, targetUserID, targetKeyID)
	if err != nil {
		return nil, err
	}
	// defer internal.CloseAndLogIfError(ctx, rows, "selectCrossSigningSigsForTargetStmt: rows.close() failed")
	r = types.CrossSigningSigMap{}
	// for rows.Next() {
	for _, item := range rows {
		var userID string
		var keyID gomatrixserverlib.KeyID
		var signature gomatrixserverlib.Base64Bytes
		// if err := rows.Scan(&userID, &keyID, &signature); err != nil {
		// 	return nil, err
		// }
		userID = item.CrossSigningSigs.OriginUserId
		keyID = gomatrixserverlib.KeyID(item.CrossSigningSigs.OriginKeyId)
		signature = gomatrixserverlib.Base64Bytes(item.CrossSigningSigs.Signature)
		if _, ok := r[userID]; !ok {
			r[userID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{}
		}
		r[userID][keyID] = signature
	}
	return
}

func (s *crossSigningSigsStatements) UpsertCrossSigningSigsForTarget(
	ctx context.Context, txn *sql.Tx,
	originUserID string, originKeyID gomatrixserverlib.KeyID,
	targetUserID string, targetKeyID gomatrixserverlib.KeyID,
	signature gomatrixserverlib.Base64Bytes,
) error {
	// 	"INSERT OR REPLACE INTO keyserver_cross_signing_sigs (origin_user_id, origin_key_id, target_user_id, target_key_id, signature)" +
	// 	" VALUES($1, $2, $3, $4, $5)"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	// 	PRIMARY KEY (origin_user_id, target_user_id, target_key_id)
	docId := fmt.Sprintf("%s_%s_%s", originUserID, targetUserID, targetKeyID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)

	dbData, _ := getCrossSigningSigs(s, ctx, pk, cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.CrossSigningSigs.OriginKeyId = string(originKeyID)
		dbData.CrossSigningSigs.Signature = signature
	} else {
		data := CrossSigningSigsCosmos{
			TargetUserId: targetUserID,
			TargetKeyId:  string(targetKeyID),
			OriginUserId: originUserID,
			OriginKeyId:  string(originKeyID),
			Signature:    signature,
		}

		dbData = &CrossSigningSigsCosmosData{
			CosmosDocument:   cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
			CrossSigningSigs: data,
		}
	}
	// if _, err := sqlutil.TxStmt(txn, s.upsertCrossSigningSigsForTargetStmt).ExecContext(ctx, originUserID, originKeyID, targetUserID, targetKeyID, signature); err != nil {
	// 	return fmt.Errorf("s.upsertCrossSigningSigsForTargetStmt: %w", err)
	// }
	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func (s *crossSigningSigsStatements) DeleteCrossSigningSigsForTarget(
	ctx context.Context, txn *sql.Tx,
	targetUserID string, targetKeyID gomatrixserverlib.KeyID,
) error {
	// 	"DELETE FROM keyserver_cross_signing_sigs WHERE target_user_id=$1 AND target_key_id=$2"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": targetUserID,
		"@x3": targetKeyID,
	}
	rows, err := queryCrossSigningSigs(s, ctx, s.selectCrossSigningSigsForTargetStmt, params)
	// if _, err := sqlutil.TxStmt(txn, s.deleteCrossSigningSigsForTargetStmt).ExecContext(ctx, targetUserID, targetKeyID); err != nil {
	// 	return fmt.Errorf("s.deleteCrossSigningSigsForTargetStmt: %w", err)
	// }
	if err != nil {
		return err
	}

	for _, item := range rows {
		errItem := deleteCrossSigningSigs(s, ctx, item)
		if errItem != nil {
			return fmt.Errorf("s.deleteCrossSigningSigsForTargetStmt: %w", err)
		}
	}
	return nil
}
