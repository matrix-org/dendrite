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

// const blacklistSchema = `
// CREATE TABLE IF NOT EXISTS federationsender_blacklist (
//     -- The blacklisted server name
// 	server_name TEXT NOT NULL,
// 	UNIQUE (server_name)
// );
// `

type blacklistCosmos struct {
	ServerName string `json:"server_name"`
}

type blacklistCosmosData struct {
	cosmosdbapi.CosmosDocument
	Blacklist blacklistCosmos `json:"mx_federationsender_blacklist"`
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

func (s *blacklistStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *blacklistStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getBlacklist(s *blacklistStatements, ctx context.Context, pk string, docId string) (*blacklistCosmosData, error) {
	response := blacklistCosmosData{}
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

func deleteBlacklist(s *blacklistStatements, ctx context.Context, dbData blacklistCosmosData) error {
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

	// 	UNIQUE (server_name)
	docId := fmt.Sprintf("%s", serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getBlacklist(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
	} else {
		data := blacklistCosmos{
			ServerName: string(serverName),
		}

		dbData = &blacklistCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			Blacklist:      data,
		}
	}

	// _, err := stmt.ExecContext(ctx, serverName)

	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		&dbData)
}

// selectRoomForUpdate locks the row for the room and returns the last_event_id.
// The row must already exist in the table. Callers can ensure that the row
// exists by calling insertRoom first.
func (s *blacklistStatements) SelectBlacklist(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (bool, error) {
	// 	"SELECT server_name FROM federationsender_blacklist WHERE server_name = $1"

	// stmt := sqlutil.TxStmt(txn, s.selectBlacklistStmt)

	// 	UNIQUE (server_name)
	docId := fmt.Sprintf("%s", serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	// res, err := stmt.QueryContext(ctx, serverName)
	res, err := getBlacklist(s, ctx, s.getPartitionKey(), cosmosDocId)
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
	// 	UNIQUE (server_name)
	docId := fmt.Sprintf("%s", serverName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	// _, err := stmt.ExecContext(ctx, serverName)
	res, err := getBlacklist(s, ctx, s.getPartitionKey(), cosmosDocId)
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
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}

	// rows, err := sqlutil.TxStmt(txn, s.selectInboundPeeksStmt).QueryContext(ctx, roomID)
	var rows []blacklistCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.deleteAllBlacklistStmt, params, &rows)

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
