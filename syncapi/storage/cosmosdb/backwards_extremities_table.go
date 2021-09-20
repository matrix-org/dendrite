// Copyright 2018 New Vector Ltd
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

	"github.com/matrix-org/dendrite/syncapi/storage/tables"
)

// const backwardExtremitiesSchema = `
// -- Stores output room events received from the roomserver.
// CREATE TABLE IF NOT EXISTS syncapi_backward_extremities (
// 	-- The 'room_id' key for the event.
// 	room_id TEXT NOT NULL,
// 	-- The event ID for the last known event. This is the backwards extremity.
// 	event_id TEXT NOT NULL,
// 	-- The prev_events for the last known event. This is used to update extremities.
// 	prev_event_id TEXT NOT NULL,
// 	PRIMARY KEY(room_id, event_id, prev_event_id)
// );
// `

type BackwardExtremityCosmos struct {
	RoomID      string `json:"room_id"`
	EventID     string `json:"event_id"`
	PrevEventID string `json:"prev_event_id"`
}

type BackwardExtremityCosmosData struct {
	cosmosdbapi.CosmosDocument
	BackwardExtremity BackwardExtremityCosmos `json:"mx_syncapi_backward_extremity"`
}

// const insertBackwardExtremitySQL = "" +
// 	"INSERT INTO syncapi_backward_extremities (room_id, event_id, prev_event_id)" +
// 	" VALUES ($1, $2, $3)" +
// 	" ON CONFLICT (room_id, event_id, prev_event_id) DO NOTHING"

// "SELECT event_id, prev_event_id FROM syncapi_backward_extremities WHERE room_id = $1"
const selectBackwardExtremitiesForRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_account_data_type.room_id = @x2 "

//  "DELETE FROM syncapi_backward_extremities WHERE room_id = $1 AND prev_event_id = $2"
const deleteBackwardExtremitySQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_account_data_type.room_id = @x2 " +
	"and c.mx_syncapi_account_data_type.prev_event_id = @x3"

// "DELETE FROM syncapi_backward_extremities WHERE room_id = $1"
const deleteBackwardExtremitiesForRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_account_data_type.room_id = @x2 "

type backwardExtremitiesStatements struct {
	db *SyncServerDatasource
	// insertBackwardExtremityStmt          *sql.Stmt
	selectBackwardExtremitiesForRoomStmt string
	deleteBackwardExtremityStmt          string
	deleteBackwardExtremitiesForRoomStmt string
	tableName                            string
}

func getBackwardExtremity(s *backwardExtremitiesStatements, ctx context.Context, pk string, docId string) (*BackwardExtremityCosmosData, error) {
	response := BackwardExtremityCosmosData{}
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

func queryBackwardExtremity(s *backwardExtremitiesStatements, ctx context.Context, qry string, params map[string]interface{}) ([]BackwardExtremityCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []BackwardExtremityCosmosData

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

func deleteBackwardExtremity(s *backwardExtremitiesStatements, ctx context.Context, dbData BackwardExtremityCosmosData) error {
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

func NewCosmosDBBackwardsExtremitiesTable(db *SyncServerDatasource) (tables.BackwardsExtremities, error) {
	s := &backwardExtremitiesStatements{
		db: db,
	}
	s.selectBackwardExtremitiesForRoomStmt = selectBackwardExtremitiesForRoomSQL
	s.deleteBackwardExtremityStmt = deleteBackwardExtremitySQL
	s.deleteBackwardExtremitiesForRoomStmt = deleteBackwardExtremitiesForRoomSQL
	s.tableName = "backward_extremities"
	return s, nil
}

func (s *backwardExtremitiesStatements) InsertsBackwardExtremity(
	ctx context.Context, txn *sql.Tx, roomID, eventID string, prevEventID string,
) (err error) {

	// "INSERT INTO syncapi_backward_extremities (room_id, event_id, prev_event_id)" +
	// " VALUES ($1, $2, $3)" +
	// " ON CONFLICT (room_id, event_id, prev_event_id) DO NOTHING"

	// _, err = sqlutil.TxStmt(txn, s.insertBackwardExtremityStmt).ExecContext(ctx, roomID, eventID, prevEventID)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	PRIMARY KEY(room_id, event_id, prev_event_id)
	docId := fmt.Sprintf("%s_%s_%s", roomID, eventID, prevEventID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	dbData, _ := getBackwardExtremity(s, ctx, pk, cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
	} else {
		data := BackwardExtremityCosmos{
			EventID:     eventID,
			PrevEventID: prevEventID,
			RoomID:      roomID,
		}

		dbData = &BackwardExtremityCosmosData{
			CosmosDocument:    cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
			BackwardExtremity: data,
		}
	}

	err = cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)

	return
}

func (s *backwardExtremitiesStatements) SelectBackwardExtremitiesForRoom(
	ctx context.Context, roomID string,
) (bwExtrems map[string][]string, err error) {

	// "SELECT event_id, prev_event_id FROM syncapi_backward_extremities WHERE room_id = $1"

	// rows, err := s.selectBackwardExtremitiesForRoomStmt.QueryContext(ctx, roomID)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	rows, err := queryBackwardExtremity(s, ctx, s.selectBackwardExtremitiesForRoomStmt, params)

	if err != nil {
		return
	}

	bwExtrems = make(map[string][]string)
	for _, item := range rows {
		var eID string
		var prevEventID string
		eID = item.BackwardExtremity.EventID
		prevEventID = item.BackwardExtremity.PrevEventID
		bwExtrems[eID] = append(bwExtrems[eID], prevEventID)
	}

	return bwExtrems, err
}

func (s *backwardExtremitiesStatements) DeleteBackwardExtremity(
	ctx context.Context, txn *sql.Tx, roomID, knownEventID string,
) (err error) {

	//  "DELETE FROM syncapi_backward_extremities WHERE room_id = $1 AND prev_event_id = $2"

	// _, err = sqlutil.TxStmt(txn, s.deleteBackwardExtremityStmt).ExecContext(ctx, roomID, knownEventID)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
		"@x3": knownEventID,
	}

	rows, err := queryBackwardExtremity(s, ctx, s.deleteBackwardExtremityStmt, params)
	if err != nil {
		return
	}

	for _, item := range rows {
		err = deleteBackwardExtremity(s, ctx, item)
	}
	return
}

func (s *backwardExtremitiesStatements) DeleteBackwardExtremitiesForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {

	// "DELETE FROM syncapi_backward_extremities WHERE room_id = $1"

	// _, err = sqlutil.TxStmt(txn, s.deleteBackwardExtremitiesForRoomStmt).ExecContext(ctx, roomID)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	rows, err := queryBackwardExtremity(s, ctx, s.deleteBackwardExtremitiesForRoomStmt, params)
	if err != nil {
		return
	}

	for _, item := range rows {
		err = deleteBackwardExtremity(s, ctx, item)
	}
	return
}
