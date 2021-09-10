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

// const blacklistSchema = `
// CREATE TABLE IF NOT EXISTS federationsender_blacklist (
//     -- The blacklisted server name
// 	server_name TEXT NOT NULL,
// 	UNIQUE (server_name)
// );
// `

type BlacklistCosmos struct {
	ServerName string `json:"server_name"`
}

type BlacklistCosmosData struct {
	Id        string          `json:"id"`
	Pk        string          `json:"_pk"`
	Tn        string          `json:"_sid"`
	Cn        string          `json:"_cn"`
	ETag      string          `json:"_etag"`
	Timestamp int64           `json:"_ts"`
	Blacklist BlacklistCosmos `json:"mx_federationsender_blacklist"`
}

// const insertBlacklistSQL = "" +
// 	"INSERT INTO federationsender_blacklist (server_name) VALUES ($1)" +
// 	" ON CONFLICT DO NOTHING"

// const selectBlacklistSQL = "" +
// 	"SELECT server_name FROM federationsender_blacklist WHERE server_name = $1"

// const deleteBlacklistSQL = "" +
// 	"DELETE FROM federationsender_blacklist WHERE server_name = $1"

// 	"DELETE FROM federationsender_blacklist"
const deleteAllBlacklistSQL = "" +
	"select * from c where c._cn = @x1 "

type blacklistStatements struct {
	db *Database
	// insertBlacklistStmt *sql.Stmt
	// selectBlacklistStmt *sql.Stmt
	// deleteBlacklistStmt *sql.Stmt
	deleteAllBlacklistStmt string
	tableName              string
}

func getBlacklist(s *blacklistStatements, ctx context.Context, pk string, docId string) (*BlacklistCosmosData, error) {
	response := BlacklistCosmosData{}
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

func queryBlacklist(s *blacklistStatements, ctx context.Context, qry string, params map[string]interface{}) ([]BlacklistCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []BlacklistCosmosData

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

func deleteBlacklist(s *blacklistStatements, ctx context.Context, dbData BlacklistCosmosData) error {
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

func NewCosmosDBBlacklistTable(db *Database) (s *blacklistStatements, err error) {
	s = &blacklistStatements{
		db: db,
	}
	s.deleteAllBlacklistStmt = deleteAllBlacklistSQL
	s.tableName = "blacklists"
	return
}

// insertRoom inserts the room if it didn't already exist.
// If the room didn't exist then last_event_id is set to the empty string.
func (s *blacklistStatements) InsertBlacklist(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) error {

	// 	"INSERT INTO federationsender_blacklist (server_name) VALUES ($1)" +
	// 	" ON CONFLICT DO NOTHING"

	// stmt := sqlutil.TxStmt(txn, s.insertBlacklistStmt)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	UNIQUE (server_name)
	docId := fmt.Sprintf("%s", serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	data := BlacklistCosmos{
		ServerName: string(serverName),
	}

	dbData := &BlacklistCosmosData{
		Id:        cosmosDocId,
		Tn:        s.db.cosmosConfig.TenantName,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		Blacklist: data,
	}

	// _, err := stmt.ExecContext(ctx, serverName)

	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
}

// selectRoomForUpdate locks the row for the room and returns the last_event_id.
// The row must already exist in the table. Callers can ensure that the row
// exists by calling insertRoom first.
func (s *blacklistStatements) SelectBlacklist(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (bool, error) {
	// 	"SELECT server_name FROM federationsender_blacklist WHERE server_name = $1"

	// stmt := sqlutil.TxStmt(txn, s.selectBlacklistStmt)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	UNIQUE (server_name)
	docId := fmt.Sprintf("%s", serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	// res, err := stmt.QueryContext(ctx, serverName)
	res, err := getBlacklist(s, ctx, pk, cosmosDocId)
	if err != nil {
		return false, err
	}
	// The query will return the server name if the server is blacklisted, and
	// will return no rows if not. By calling Next, we find out if a row was
	// returned or not - we don't care about the value itself.
	return res != nil, nil
}

// updateRoom updates the last_event_id for the room. selectRoomForUpdate should
// have already been called earlier within the transaction.
func (s *blacklistStatements) DeleteBlacklist(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) error {
	// 	"DELETE FROM federationsender_blacklist WHERE server_name = $1"

	// stmt := sqlutil.TxStmt(txn, s.deleteBlacklistStmt)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	UNIQUE (server_name)
	docId := fmt.Sprintf("%s", serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	// _, err := stmt.ExecContext(ctx, serverName)
	res, err := getBlacklist(s, ctx, pk, cosmosDocId)
	if res != nil {
		_ = deleteBlacklist(s, ctx, *res)
	}
	return err
}

func (s *blacklistStatements) DeleteAllBlacklist(
	ctx context.Context, txn *sql.Tx,
) error {
	// 	"DELETE FROM federationsender_blacklist"

	// stmt := sqlutil.TxStmt(txn, s.deleteAllBlacklistStmt)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	// rows, err := sqlutil.TxStmt(txn, s.selectInboundPeeksStmt).QueryContext(ctx, roomID)
	rows, err := queryBlacklist(s, ctx, s.deleteAllBlacklistStmt, params)

	if err != nil {
		return err
	}
	// _, err := stmt.ExecContext(ctx)
	for _, item := range rows {
		// stmt := sqlutil.TxStmt(txn, deleteStmt)
		err = deleteBlacklist(s, ctx, item)
		if err != nil {
			return err
		}
	}
	return err
}
