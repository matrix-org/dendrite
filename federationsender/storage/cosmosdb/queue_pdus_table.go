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

	"github.com/matrix-org/gomatrixserverlib"
)

// const queuePDUsSchema = `
// CREATE TABLE IF NOT EXISTS federationsender_queue_pdus (
//     -- The transaction ID that was generated before persisting the event.
// 	transaction_id TEXT NOT NULL,
//     -- The domain part of the user ID the m.room.member event is for.
// 	server_name TEXT NOT NULL,
// 	-- The JSON NID from the federationsender_queue_pdus_json table.
// 	json_nid BIGINT NOT NULL
// );

// CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_pdus_pdus_json_nid_idx
//     ON federationsender_queue_pdus (json_nid, server_name);
// `

type QueuePDUCosmos struct {
	TransactionID string `json:"transaction_id"`
	ServerName    string `json:"server_name"`
	JSONNID       int64  `json:"json_nid"`
}

type QueuePDUCosmosNumber struct {
	Number int64 `json:"number"`
}

type QueuePDUCosmosData struct {
	cosmosdbapi.CosmosDocument
	QueuePDU QueuePDUCosmos `json:"mx_federationsender_queue_pdu"`
}

// const insertQueuePDUSQL = "" +
// 	"INSERT INTO federationsender_queue_pdus (transaction_id, server_name, json_nid)" +
// 	" VALUES ($1, $2, $3)"

// "DELETE FROM federationsender_queue_pdus WHERE server_name = $1 AND json_nid IN ($2)"
const deleteQueuePDUsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_pdu.server_name = @x2 " +
	"and ARRAY_CONTAINS(@x3, c.mx_federationsender_queue_pdu.json_nid) "

// "SELECT transaction_id FROM federationsender_queue_pdus" +
// " WHERE server_name = $1" +
// " ORDER BY transaction_id ASC" +
// " LIMIT 1"
const selectQueueNextTransactionIDSQL = "" +
	"select top 1 * from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_pdu.server_name = @x2 " +
	"order by c.mx_federationsender_queue_pdu.transaction_id asc "

// "SELECT json_nid FROM federationsender_queue_pdus" +
// " WHERE server_name = $1" +
// " LIMIT $2"
const selectQueuePDUsSQL = "" +
	"select top @x3 * from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_pdu.server_name = @x2 "

// "SELECT COUNT(*) FROM federationsender_queue_pdus" +
// " WHERE json_nid = $1"
const selectQueuePDUsReferenceJSONCountSQL = "" +
	"select count(c._ts) as number from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_pdu.json_nid = @x2 "

// "SELECT COUNT(*) FROM federationsender_queue_pdus" +
// " WHERE server_name = $1"
const selectQueuePDUsCountSQL = "" +
	"select count(c._ts) as number from c where c._cn = @x1 " +
	"and c.mx_federationsender_queue_pdu.server_name = @x2 "

// "SELECT DISTINCT server_name FROM federationsender_queue_pdus"
const selectQueuePDUsServerNamesSQL = "" +
	"select distinct c.mx_federationsender_queue_pdu.server_name from c where c._cn = @x1 "

type queuePDUsStatements struct {
	db *Database
	// insertQueuePDUStmt                *sql.Stmt
	selectQueueNextTransactionIDStmt  string
	selectQueuePDUsStmt               string
	selectQueueReferenceJSONCountStmt string
	selectQueuePDUsCountStmt          string
	selectQueueServerNamesStmt        string
	// deleteQueuePDUsStmt *sql.Stmt - prepared at runtime due to variadic
	tableName string
}

func queryQueuePDU(s *queuePDUsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]QueuePDUCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []QueuePDUCosmosData

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

func queryQueuePDUDistinct(s *queuePDUsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]QueuePDUCosmos, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []QueuePDUCosmos

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

func queryQueuePDUNumber(s *queuePDUsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]QueuePDUCosmosNumber, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []QueuePDUCosmosNumber

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

func deleteQueuePDU(s *queuePDUsStatements, ctx context.Context, dbData QueuePDUCosmosData) error {
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

func NewCosmosDBQueuePDUsTable(db *Database) (s *queuePDUsStatements, err error) {
	s = &queuePDUsStatements{
		db: db,
	}
	s.selectQueueNextTransactionIDStmt = selectQueueNextTransactionIDSQL
	s.selectQueuePDUsStmt = selectQueuePDUsSQL
	s.selectQueueReferenceJSONCountStmt = selectQueuePDUsReferenceJSONCountSQL
	s.selectQueuePDUsCountStmt = selectQueuePDUsCountSQL
	s.selectQueueServerNamesStmt = selectQueuePDUsServerNamesSQL
	s.tableName = "queue_pdus"
	return
}

func (s *queuePDUsStatements) InsertQueuePDU(
	ctx context.Context,
	txn *sql.Tx,
	transactionID gomatrixserverlib.TransactionID,
	serverName gomatrixserverlib.ServerName,
	nid int64,
) error {

	// "INSERT INTO federationsender_queue_pdus (transaction_id, server_name, json_nid)" +
	// " VALUES ($1, $2, $3)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_pdus_pdus_json_nid_idx
	//     ON federationsender_queue_pdus (json_nid, server_name);
	docId := fmt.Sprintf("%d_%s", nid, serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	data := QueuePDUCosmos{
		JSONNID:       nid,
		ServerName:    string(serverName),
		TransactionID: string(transactionID),
	}

	dbData := &QueuePDUCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
		QueuePDU:       data,
	}

	// stmt := sqlutil.TxStmt(txn, s.insertQueuePDUStmt)
	// _, err := stmt.ExecContext(
	// 	ctx,
	// 	transactionID, // the transaction ID that we initially attempted
	// 	serverName,    // destination server name
	// 	nid,           // JSON blob NID
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

func (s *queuePDUsStatements) DeleteQueuePDUs(
	ctx context.Context, txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	jsonNIDs []int64,
) error {

	// "DELETE FROM federationsender_queue_pdus WHERE server_name = $1 AND json_nid IN ($2)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": serverName,
		"@x3": jsonNIDs,
	}

	// deleteSQL := strings.Replace(deleteQueuePDUsSQL, "($2)", sqlutil.QueryVariadicOffset(len(jsonNIDs), 1), 1)
	// deleteStmt, err := txn.Prepare(deleteSQL)
	rows, err := queryQueuePDU(s, ctx, deleteQueuePDUsSQL, params)

	if err != nil {
		return err
	}

	for _, item := range rows {
		// stmt := sqlutil.TxStmt(txn, deleteStmt)
		err = deleteQueuePDU(s, ctx, item)
		if err != nil {
			return err
		}
	}
	return err
}

func (s *queuePDUsStatements) SelectQueuePDUNextTransactionID(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (gomatrixserverlib.TransactionID, error) {
	var transactionID gomatrixserverlib.TransactionID

	// "SELECT transaction_id FROM federationsender_queue_pdus" +
	// " WHERE server_name = $1" +
	// " ORDER BY transaction_id ASC" +
	// " LIMIT 1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": serverName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueNextTransactionIDStmt)
	rows, err := queryQueuePDU(s, ctx, s.selectQueueNextTransactionIDStmt, params)

	if err != nil {
		return "", err
	}

	if len(rows) == 0 {
		return "", nil
	}
	// err := stmt.QueryRowContext(ctx, serverName).Scan(&transactionID)
	transactionID = gomatrixserverlib.TransactionID(rows[0].QueuePDU.TransactionID)
	return transactionID, err
}

func (s *queuePDUsStatements) SelectQueuePDUReferenceJSONCount(
	ctx context.Context, txn *sql.Tx, jsonNID int64,
) (int64, error) {
	var count int64

	// "SELECT COUNT(*) FROM federationsender_queue_pdus" +
	// " WHERE json_nid = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": jsonNID,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueReferenceJSONCountStmt)
	rows, err := queryQueuePDUNumber(s, ctx, s.selectQueueReferenceJSONCountStmt, params)

	if err != nil {
		return -1, err
	}

	if len(rows) == 0 {
		return -1, nil
	}
	// err := stmt.QueryRowContext(ctx, jsonNID).Scan(&count)
	count = rows[0].Number
	return count, err
}

func (s *queuePDUsStatements) SelectQueuePDUCount(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (int64, error) {
	var count int64

	// "SELECT COUNT(*) FROM federationsender_queue_pdus" +
	// " WHERE server_name = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": serverName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueuePDUsCountStmt)
	rows, err := queryQueuePDUNumber(s, ctx, s.selectQueuePDUsCountStmt, params)

	if err != nil {
		return 0, err
	}

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

func (s *queuePDUsStatements) SelectQueuePDUs(
	ctx context.Context, txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	limit int,
) ([]int64, error) {

	// "SELECT json_nid FROM federationsender_queue_pdus" +
	// " WHERE server_name = $1" +
	// " LIMIT $2"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": serverName,
		"@x3": limit,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueuePDUsStmt)
	// rows, err := stmt.QueryContext(ctx, serverName, limit)
	rows, err := queryQueuePDU(s, ctx, s.selectQueuePDUsStmt, params)

	if err != nil {
		return nil, err
	}
	var result []int64
	for _, item := range rows {
		var nid int64
		nid = item.QueuePDU.JSONNID
		result = append(result, nid)
	}

	return result, nil
}

func (s *queuePDUsStatements) SelectQueuePDUServerNames(
	ctx context.Context, txn *sql.Tx,
) ([]gomatrixserverlib.ServerName, error) {

	// "SELECT DISTINCT server_name FROM federationsender_queue_pdus"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueServerNamesStmt)
	// rows, err := stmt.QueryContext(ctx)
	rows, err := queryQueuePDUDistinct(s, ctx, s.selectQueueServerNamesStmt, params)
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
