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

type queueEDUCosmos struct {
	EDUType    string `json:"edu_type"`
	ServerName string `json:"server_name"`
	JSONNID    int64  `json:"json_nid"`
}

type queueEDUCosmosNumber struct {
	Number int64 `json:"number"`
}

type queueEDUCosmosData struct {
	cosmosdbapi.CosmosDocument
	QueueEDU queueEDUCosmos `json:"mx_federationsender_queue_edu"`
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

func (s *queueEDUsStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *queueEDUsStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func deleteQueueEDUC(s *queueEDUsStatements, ctx context.Context, dbData queueEDUCosmosData) error {
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

	// CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_edus_json_nid_idx
	//     ON federationsender_queue_edus (json_nid, server_name);
	docId := fmt.Sprintf("%d_%s", nid, eduType)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := queueEDUCosmos{
		EDUType:    eduType,
		JSONNID:    nid,
		ServerName: string(serverName),
	}

	dbData := &queueEDUCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		QueueEDU:       data,
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": serverName,
		"@x3": jsonNIDs,
	}

	// stmt := sqlutil.TxStmt(txn, deleteStmt)
	// _, err = stmt.ExecContext(ctx, params...)
	var rows []queueEDUCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), deleteQueueEDUsSQL, params, &rows)

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
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": serverName,
		"@x3": limit,
	}

	// rows, err := stmt.QueryContext(ctx, serverName, limit)
	var rows []queueEDUCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), deleteQueueEDUsSQL, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": jsonNID,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueEDUReferenceJSONCountStmt)
	var rows []queueEDUCosmosNumber
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueueEDUReferenceJSONCountStmt, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": serverName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueEDUCountStmt)
	var rows []queueEDUCosmosNumber
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueueEDUCountStmt, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueEDUServerNamesStmt)
	// rows, err := stmt.QueryContext(ctx)
	var rows []queueEDUCosmos
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueueEDUServerNamesStmt, params, &rows)

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
