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

type queuePDUCosmos struct {
	TransactionID string `json:"transaction_id"`
	ServerName    string `json:"server_name"`
	JSONNID       int64  `json:"json_nid"`
}

type queuePDUCosmosNumber struct {
	Number int64 `json:"number"`
}

type queuePDUCosmosData struct {
	cosmosdbapi.CosmosDocument
	QueuePDU queuePDUCosmos `json:"mx_federationsender_queue_pdu"`
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

func (s *queuePDUsStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *queuePDUsStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func deleteQueuePDU(s *queuePDUsStatements, ctx context.Context, dbData queuePDUCosmosData) error {
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

	// CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_pdus_pdus_json_nid_idx
	//     ON federationsender_queue_pdus (json_nid, server_name);
	docId := fmt.Sprintf("%d,%s", nid, serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := queuePDUCosmos{
		JSONNID:       nid,
		ServerName:    string(serverName),
		TransactionID: string(transactionID),
	}

	dbData := &queuePDUCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": serverName,
		"@x3": jsonNIDs,
	}

	// deleteSQL := strings.Replace(deleteQueuePDUsSQL, "($2)", sqlutil.QueryVariadicOffset(len(jsonNIDs), 1), 1)
	// deleteStmt, err := txn.Prepare(deleteSQL)
	var rows []queuePDUCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), deleteQueuePDUsSQL, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": serverName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueNextTransactionIDStmt)
	var rows []queuePDUCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueueNextTransactionIDStmt, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": jsonNID,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueReferenceJSONCountStmt)
	var rows []queuePDUCosmosNumber
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueueReferenceJSONCountStmt, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": serverName,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueuePDUsCountStmt)
	var rows []queuePDUCosmosNumber
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueuePDUsCountStmt, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": serverName,
		"@x3": limit,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueuePDUsStmt)
	// rows, err := stmt.QueryContext(ctx, serverName, limit)
	var rows []queuePDUCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueuePDUsStmt, params, &rows)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}

	// stmt := sqlutil.TxStmt(txn, s.selectQueueServerNamesStmt)
	// rows, err := stmt.QueryContext(ctx)
	var rows []queuePDUCosmos
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectQueueServerNamesStmt, params, &rows)

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
