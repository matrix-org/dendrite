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

	"github.com/matrix-org/gomatrixserverlib"
)

// const queueEDUsSchema = `
// CREATE TABLE IF NOT EXISTS federationsender_queue_edus (
// 	-- The type of the event (informational).
// 	edu_type TEXT NOT NULL,
//     -- The domain part of the user ID the EDU event is for.
// 	server_name TEXT NOT NULL,
// 	-- The JSON NID from the federationsender_queue_edus_json table.
// 	json_nid BIGINT NOT NULL
// );

// CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_edus_json_nid_idx
//     ON federationsender_queue_edus (json_nid, server_name);
// `

type QueueEDUCosmos struct {
	EDUType    string `json:"edu_type"`
	ServerName string `json:"server_name"`
	JSONNID    int64  `json:"json_nid"`
}

type QueueEDUCosmosNumber struct {
	Number int64 `json:"number"`
}

type QueueEDUCosmosData struct {
	Id        string         `json:"id"`
	Pk        string         `json:"_pk"`
	Cn        string         `json:"_cn"`
	ETag      string         `json:"_etag"`
	Timestamp int64          `json:"_ts"`
	QueueEDU  QueueEDUCosmos `json:"mx_federationsender_queue_edu"`
}

// const insertQueueEDUSQL = "" +
// 	"INSERT INTO federationsender_queue_edus (edu_type, server_name, json_nid)" +
// 	" VALUES ($1, $2, $3)"

// "DELETE FROM federationsender_queue_edus WHERE server_name = $1 AND json_nid IN ($2)"
const deleteQueueEDUsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_edu.server_name = @x2" +
	"and ARRAY_CONTAINS(@x3, c.mx_federationsender_queue_edu.json_nid) "

// "SELECT json_nid FROM federationsender_queue_edus" +
// " WHERE server_name = $1" +
// " LIMIT $2"
const selectQueueEDUSQL = "" +
	"select top @x3 * from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_edu.server_name = @x2"

// "SELECT COUNT(*) FROM federationsender_queue_edus" +
// " WHERE json_nid = $1"
const selectQueueEDUReferenceJSONCountSQL = "" +
	"select count(c._ts) as number from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_edu.json_nid = @x2"

// "SELECT COUNT(*) FROM federationsender_queue_edus" +
// " WHERE server_name = $1"
const selectQueueEDUCountSQL = "" +
	"select count(c._ts) as number from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_edu.server_name = @x2"

// "SELECT DISTINCT server_name FROM federationsender_queue_edus"
const selectQueueServerNamesSQL = "" +
	"select distinct c.mx_federationsender_queue_edu.server_name from c where c._cn = @x1 "

type queueEDUsStatements struct {
	db *Database
	// insertQueueEDUStmt                   *sql.Stmt
	selectQueueEDUStmt                   string
	selectQueueEDUReferenceJSONCountStmt string
	selectQueueEDUCountStmt              string
	selectQueueEDUServerNamesStmt        string
	tableName                            string
}

func queryQueueEDUC(s *queueEDUsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]QueueEDUCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []QueueEDUCosmosData

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

func queryQueueEDUCDistinct(s *queueEDUsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]QueueEDUCosmos, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []QueueEDUCosmos

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

func queryQueueEDUCNumber(s *queueEDUsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]QueueEDUCosmosNumber, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []QueueEDUCosmosNumber

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

func deleteQueueEDUC(s *queueEDUsStatements, ctx context.Context, dbData QueueEDUCosmosData) error {
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

func NewCosmosDBQueueEDUsTable(db *Database) (s *queueEDUsStatements, err error) {
	s = &queueEDUsStatements{
		db: db,
	}
	s.selectQueueEDUStmt = selectQueueEDUSQL
	s.selectQueueEDUReferenceJSONCountStmt = selectQueueEDUReferenceJSONCountSQL
	s.selectQueueEDUCountStmt = selectQueueEDUCountSQL
	s.selectQueueEDUServerNamesStmt = selectQueueServerNamesSQL
	s.tableName = "queue_edus"
	return
}

func (s *queueEDUsStatements) InsertQueueEDU(
	ctx context.Context,
	txn *sql.Tx,
	eduType string,
	serverName gomatrixserverlib.ServerName,
	nid int64,
) error {

	// 	"INSERT INTO federationsender_queue_edus (edu_type, server_name, json_nid)" +

	// stmt := sqlutil.TxStmt(txn, s.insertQueueEDUStmt)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_edus_json_nid_idx
	//     ON federationsender_queue_edus (json_nid, server_name);
	docId := fmt.Sprintf("%d_%s", nid, eduType)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	data := QueueEDUCosmos{
		EDUType:    eduType,
		JSONNID:    nid,
		ServerName: string(serverName),
	}

	dbData := &QueueEDUCosmosData{
		Id:        cosmosDocId,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		QueueEDU:  data,
	}

	// _, err := stmt.ExecContext(
	// 	ctx,
	// 	eduType,    // the EDU type
	// 	serverName, // destination server name
	// 	nid,        // JSON blob NID
	// )

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
}

func (s *queueEDUsStatements) DeleteQueueEDUs(
	ctx context.Context, txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	jsonNIDs []int64,
) error {

	// "DELETE FROM federationsender_queue_edus WHERE server_name = $1 AND json_nid IN ($2)"

	// deleteSQL := strings.Replace(deleteQueueEDUsSQL, "($2)", sqlutil.QueryVariadicOffset(len(jsonNIDs), 1), 1)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": serverName,
		"@x3": jsonNIDs,
	}

	// stmt := sqlutil.TxStmt(txn, deleteStmt)
	// _, err = stmt.ExecContext(ctx, params...)
	rows, err := queryQueueEDUC(s, ctx, deleteQueueEDUsSQL, params)

	if err != nil {
		return err
	}

	for _, item := range rows {
		err = deleteQueueEDUC(s, ctx, item)
		if err != nil {
			return err
		}
	}

	return err
}

func (s *queueEDUsStatements) SelectQueueEDUs(
	ctx context.Context, txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	limit int,
) ([]int64, error) {

	// "SELECT json_nid FROM federationsender_queue_edus" +
	// " WHERE server_name = $1" +
	// " LIMIT $2"

	// stmt := sqlutil.TxStmt(txn, s.selectQueueEDUStmt)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": serverName,
		"@x3": limit,
	}

	// rows, err := stmt.QueryContext(ctx, serverName, limit)
	rows, err := queryQueueEDUC(s, ctx, deleteQueueEDUsSQL, params)
	if err != nil {
		return nil, err
	}
	var result []int64
	for _, item := range rows {
		var nid int64
		nid = item.QueueEDU.JSONNID
		result = append(result, nid)
	}
	return result, nil
}

func (s *queueEDUsStatements) SelectQueueEDUReferenceJSONCount(
	ctx context.Context, txn *sql.Tx, jsonNID int64,
) (int64, error) {
	var count int64

	// "SELECT COUNT(*) FROM federationsender_queue_edus" +
	// " WHERE json_nid = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": jsonNID,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueEDUReferenceJSONCountStmt)
	rows, err := queryQueueEDUCNumber(s, ctx, s.selectQueueEDUReferenceJSONCountStmt, params)
	if len(rows) == 0 {
		return -1, nil
	}
	// err := stmt.QueryRowContext(ctx, jsonNID).Scan(&count)
	count = rows[0].Number
	return count, err
}

func (s *queueEDUsStatements) SelectQueueEDUCount(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (int64, error) {
	var count int64

	// "SELECT COUNT(*) FROM federationsender_queue_edus" +
	// " WHERE server_name = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": serverName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueEDUCountStmt)
	rows, err := queryQueueEDUCNumber(s, ctx, s.selectQueueEDUCountStmt, params)
	if len(rows) == 0 {
		// It's acceptable for there to be no rows referencing a given
		// JSON NID but it's not an error condition. Just return as if
		// there's a zero count.
		return 0, nil
	}
	// err := stmt.QueryRowContext(ctx, serverName).Scan(&count)
	count = rows[0].Number
	return count, err
}

func (s *queueEDUsStatements) SelectQueueEDUServerNames(
	ctx context.Context, txn *sql.Tx,
) ([]gomatrixserverlib.ServerName, error) {

	// "SELECT DISTINCT server_name FROM federationsender_queue_edus"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueEDUServerNamesStmt)
	// rows, err := stmt.QueryContext(ctx)
	rows, err := queryQueueEDUCDistinct(s, ctx, s.selectQueueEDUServerNamesStmt, params)
	if err != nil {
		return nil, err
	}
	var result []gomatrixserverlib.ServerName
	for _, item := range rows {
		var serverName gomatrixserverlib.ServerName
		serverName = gomatrixserverlib.ServerName(item.ServerName)
		result = append(result, serverName)
	}

	return result, nil
}
