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

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/gomatrixserverlib"
)

// const joinedHostsSchema = `
// -- The joined_hosts table stores a list of m.room.member event ids in the
// -- current state for each room where the membership is "join".
// -- There will be an entry for every user that is joined to the room.
// CREATE TABLE IF NOT EXISTS federationsender_joined_hosts (
//     -- The string ID of the room.
//     room_id TEXT NOT NULL,
//     -- The event ID of the m.room.member join event.
//     event_id TEXT NOT NULL,
//     -- The domain part of the user ID the m.room.member event is for.
//     server_name TEXT NOT NULL
// );

// CREATE UNIQUE INDEX IF NOT EXISTS federatonsender_joined_hosts_event_id_idx
//     ON federationsender_joined_hosts (event_id);

// CREATE INDEX IF NOT EXISTS federatonsender_joined_hosts_room_id_idx
//     ON federationsender_joined_hosts (room_id)
// `

type JoinedHostCosmos struct {
	RoomID     string `json:"room_id"`
	EventID    string `json:"event_id"`
	ServerName string `json:"server_name"`
}

type JoinedHostCosmosData struct {
	cosmosdbapi.CosmosDocument
	JoinedHost JoinedHostCosmos `json:"mx_federationsender_joined_host"`
}

// const insertJoinedHostsSQL = "" +
// 	"INSERT OR IGNORE INTO federationsender_joined_hosts (room_id, event_id, server_name)" +
// 	" VALUES ($1, $2, $3)"

// "DELETE FROM federationsender_joined_hosts WHERE event_id = $1"
const deleteJoinedHostsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_joined_host.event_id = @x2 "

// "DELETE FROM federationsender_joined_hosts WHERE room_id = $1"
const deleteJoinedHostsForRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_joined_host.room_id = @x2 "

// "SELECT event_id, server_name FROM federationsender_joined_hosts" +
// " WHERE room_id = $1"
const selectJoinedHostsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_joined_host.room_id = @x2 "

// "SELECT DISTINCT server_name FROM federationsender_joined_hosts"
const selectAllJoinedHostsSQL = "" +
	"select distinct c.mx_federationsender_joined_host.server_name from c where c._cn = @x1 "

// "SELECT DISTINCT server_name FROM federationsender_joined_hosts WHERE room_id IN ($1)"
const selectJoinedHostsForRoomsSQL = "" +
	"select distinct c.mx_federationsender_joined_host.server_name from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_federationsender_joined_host.room_id) "

type joinedHostsStatements struct {
	db *Database
	// insertJoinedHostsStmt        *sql.Stmt
	deleteJoinedHostsStmt        string
	deleteJoinedHostsForRoomStmt string
	selectJoinedHostsStmt        string
	selectAllJoinedHostsStmt     string
	// selectJoinedHostsForRoomsStmt *sql.Stmt - prepared at runtime due to variadic
	tableName string
}

func getJoinedHost(s *joinedHostsStatements, ctx context.Context, pk string, docId string) (*JoinedHostCosmosData, error) {
	response := JoinedHostCosmosData{}
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

func queryJoinedHostDistinct(s *joinedHostsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]JoinedHostCosmos, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []JoinedHostCosmos

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

func queryJoinedHost(s *joinedHostsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]JoinedHostCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []JoinedHostCosmosData

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

func deleteJoinedHost(s *joinedHostsStatements, ctx context.Context, dbData JoinedHostCosmosData) error {
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

func NewCosmosDBJoinedHostsTable(db *Database) (s *joinedHostsStatements, err error) {
	s = &joinedHostsStatements{
		db: db,
	}
	s.deleteJoinedHostsStmt = deleteJoinedHostsSQL
	s.deleteJoinedHostsForRoomStmt = deleteJoinedHostsForRoomSQL
	s.selectJoinedHostsStmt = selectJoinedHostsSQL
	s.selectAllJoinedHostsStmt = selectAllJoinedHostsSQL
	s.tableName = "joined_hosts"
	return
}

func (s *joinedHostsStatements) InsertJoinedHosts(
	ctx context.Context,
	txn *sql.Tx,
	roomID, eventID string,
	serverName gomatrixserverlib.ServerName,
) error {

	// 	"INSERT OR IGNORE INTO federationsender_joined_hosts (room_id, event_id, server_name)" +
	// 	" VALUES ($1, $2, $3)"

	// stmt := sqlutil.TxStmt(txn, s.insertJoinedHostsStmt)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// CREATE UNIQUE INDEX IF NOT EXISTS federatonsender_joined_hosts_event_id_idx
	//     ON federationsender_joined_hosts (event_id);
	docId := fmt.Sprintf("%s", eventID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	dbData, _ := getJoinedHost(s, ctx, pk, cosmosDocId)
	if dbData == nil {
		data := JoinedHostCosmos{
			EventID:    eventID,
			RoomID:     roomID,
			ServerName: string(serverName),
		}

		dbData = &JoinedHostCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
			JoinedHost:     data,
		}
		// _, err := stmt.ExecContext(ctx, roomID, eventID, serverName)

		return cosmosdbapi.UpsertDocument(ctx,
			s.db.connection,
			s.db.cosmosConfig.DatabaseName,
			s.db.cosmosConfig.ContainerName,
			dbData.Pk,
			&dbData)
	}
	return nil
}

func (s *joinedHostsStatements) DeleteJoinedHosts(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) error {
	for _, eventID := range eventIDs {
		// "DELETE FROM federationsender_joined_hosts WHERE event_id = $1"

		var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
		params := map[string]interface{}{
			"@x1": dbCollectionName,
			"@x2": eventID,
		}
		// stmt := sqlutil.TxStmt(txn, s.deleteJoinedHostsStmt)

		rows, err := queryJoinedHost(s, ctx, s.deleteJoinedHostsStmt, params)

		for _, item := range rows {
			if err = deleteJoinedHost(s, ctx, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *joinedHostsStatements) DeleteJoinedHostsForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	// "DELETE FROM federationsender_joined_hosts WHERE room_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	// stmt := sqlutil.TxStmt(txn, s.deleteJoinedHostsForRoomStmt)
	rows, err := queryJoinedHost(s, ctx, s.deleteJoinedHostsStmt, params)

	// _, err := stmt.ExecContext(ctx, roomID)
	for _, item := range rows {
		if err = deleteJoinedHost(s, ctx, item); err != nil {
			return err
		}
	}
	return err
}

func (s *joinedHostsStatements) SelectJoinedHostsWithTx(
	ctx context.Context, txn *sql.Tx, roomID string,
) ([]types.JoinedHost, error) {
	// "SELECT event_id, server_name FROM federationsender_joined_hosts" +
	// " WHERE room_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	// stmt := sqlutil.TxStmt(txn, s.selectJoinedHostsStmt)
	rows, err := queryJoinedHost(s, ctx, s.deleteJoinedHostsStmt, params)

	if err != nil {
		return nil, err
	}

	return rowsToJoinedHosts(&rows), nil
}

func (s *joinedHostsStatements) SelectJoinedHosts(
	ctx context.Context, roomID string,
) ([]types.JoinedHost, error) {
	return s.SelectJoinedHostsWithTx(ctx, nil, roomID)
}

func (s *joinedHostsStatements) SelectAllJoinedHosts(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	// "SELECT DISTINCT server_name FROM federationsender_joined_hosts"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	// rows, err := s.selectAllJoinedHostsStmt.QueryContext(ctx)
	rows, err := queryJoinedHostDistinct(s, ctx, s.selectAllJoinedHostsStmt, params)
	if err != nil {
		return nil, err
	}

	var result []gomatrixserverlib.ServerName
	for _, item := range rows {
		var serverName string
		serverName = item.ServerName
		result = append(result, gomatrixserverlib.ServerName(serverName))
	}

	return result, err
}

func (s *joinedHostsStatements) SelectJoinedHostsForRooms(
	ctx context.Context, roomIDs []string,
) ([]gomatrixserverlib.ServerName, error) {
	// iRoomIDs := make([]interface{}, len(roomIDs))
	// for i := range roomIDs {
	// 	iRoomIDs[i] = roomIDs[i]
	// }

	// "SELECT DISTINCT server_name FROM federationsender_joined_hosts WHERE room_id IN ($1)"

	// sql := strings.Replace(selectJoinedHostsForRoomsSQL, "($1)", sqlutil.QueryVariadic(len(iRoomIDs)), 1)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomIDs,
	}

	// rows, err := s.db.QueryContext(ctx, sql, iRoomIDs...)
	rows, err := queryJoinedHostDistinct(s, ctx, s.selectAllJoinedHostsStmt, params)
	if err != nil {
		return nil, err
	}

	var result []gomatrixserverlib.ServerName
	for _, item := range rows {
		var serverName string
		serverName = item.ServerName
		result = append(result, gomatrixserverlib.ServerName(serverName))
	}

	return result, nil
}

func rowsToJoinedHosts(rows *[]JoinedHostCosmosData) []types.JoinedHost {
	var result []types.JoinedHost
	if rows == nil {
		return result
	}
	for _, item := range *rows {
		result = append(result, types.JoinedHost{
			MemberEventID: item.JoinedHost.EventID,
			ServerName:    gomatrixserverlib.ServerName(item.JoinedHost.ServerName),
		})
	}
	return result
}
