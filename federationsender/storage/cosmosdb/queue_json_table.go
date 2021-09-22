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
)

// const queueJSONSchema = `
// -- The queue_retry_json table contains event contents that
// -- we failed to send.
// CREATE TABLE IF NOT EXISTS federationsender_queue_json (
// 	-- The JSON NID. This allows the federationsender_queue_retry table to
// 	-- cross-reference to find the JSON blob.
// 	json_nid INTEGER PRIMARY KEY AUTOINCREMENT,
// 	-- The JSON body. Text so that we preserve UTF-8.
// 	json_body TEXT NOT NULL
// );
// `

type queueJSONCosmos struct {
	JSONNID  int64  `json:"json_nid"`
	JSONBody []byte `json:"json_body"`
}

type queueJSONCosmosData struct {
	cosmosdbapi.CosmosDocument
	QueueJSON queueJSONCosmos `json:"mx_federationsender_queue_json"`
}

// const insertJSONSQL = "" +
// 	"INSERT INTO federationsender_queue_json (json_body)" +
// 	" VALUES ($1)"

// "DELETE FROM federationsender_queue_json WHERE json_nid IN ($1)"
const deleteJSONSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_federationsender_queue_json.json_nid) "

// "SELECT json_nid, json_body FROM federationsender_queue_json" +
// " WHERE json_nid IN ($1)"
const selectJSONSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_federationsender_queue_json.json_nid) "

type queueJSONStatements struct {
	db *Database
	// insertJSONStmt *sql.Stmt
	//deleteJSONStmt *sql.Stmt - prepared at runtime due to variadic
	//selectJSONStmt *sql.Stmt - prepared at runtime due to variadic
	tableName string
}

func (s *queueJSONStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *queueJSONStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func deleteQueueJSON(s *queueJSONStatements, ctx context.Context, dbData queueJSONCosmosData) error {
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

func NewCosmosDBQueueJSONTable(db *Database) (s *queueJSONStatements, err error) {
	s = &queueJSONStatements{
		db: db,
	}
	s.tableName = "queue_jsons"
	return
}

func (s *queueJSONStatements) InsertQueueJSON(
	ctx context.Context, txn *sql.Tx, json string,
) (lastid int64, err error) {

	// "INSERT INTO federationsender_queue_json (json_body)" +
	// " VALUES ($1)"

	// 	json_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	idSeq, err := GetNextQueueJSONNID(s, ctx)

	// 	json_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	docId := fmt.Sprintf("%d", idSeq)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	//Convert to byte
	jsonData := []byte(json)

	data := queueJSONCosmos{
		JSONNID:  idSeq,
		JSONBody: jsonData,
	}

	dbData := &queueJSONCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		QueueJSON:      data,
	}

	// stmt := sqlutil.TxStmt(txn, s.insertJSONStmt)
	// res, err := stmt.ExecContext(ctx, json)

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	if err != nil {
		return 0, fmt.Errorf("stmt.QueryContext: %w", err)
	}
	lastid = idSeq
	return
}

func (s *queueJSONStatements) DeleteQueueJSON(
	ctx context.Context, txn *sql.Tx, nids []int64,
) error {

	// "DELETE FROM federationsender_queue_json WHERE json_nid IN ($1)"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": nids,
	}

	// deleteSQL := strings.Replace(deleteJSONSQL, "($1)", sqlutil.QueryVariadic(len(nids)), 1)
	// deleteStmt, err := txn.Prepare(deleteSQL)
	// stmt := sqlutil.TxStmt(txn, deleteStmt)
	var rows []queueJSONCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), deleteJSONSQL, params, &rows)

	if err != nil {
		return err
	}

	// iNIDs := make([]interface{}, len(nids))
	// for k, v := range nids {
	// 	iNIDs[k] = v
	// }

	for _, item := range rows {
		err = deleteQueueJSON(s, ctx, item)
	}
	return err
}

func (s *queueJSONStatements) SelectQueueJSON(
	ctx context.Context, txn *sql.Tx, jsonNIDs []int64,
) (map[int64][]byte, error) {

	// "SELECT json_nid, json_body FROM federationsender_queue_json" +
	// " WHERE json_nid IN ($1)"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": jsonNIDs,
	}

	// selectSQL := strings.Replace(selectJSONSQL, "($1)", sqlutil.QueryVariadic(len(jsonNIDs)), 1)
	// selectStmt, err := txn.Prepare(selectSQL)
	var rows []queueJSONCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectJSONSQL, params, &rows)

	if err != nil {
		return nil, fmt.Errorf("s.selectQueueJSON stmt.QueryContext: %w", err)
	}

	iNIDs := make([]interface{}, len(jsonNIDs))
	for k, v := range jsonNIDs {
		iNIDs[k] = v
	}

	blobs := map[int64][]byte{}
	for _, item := range rows {
		var nid int64
		var blob []byte
		nid = item.QueueJSON.JSONNID
		blob = item.QueueJSON.JSONBody
		blobs[nid] = blob
	}
	return blobs, err
}
