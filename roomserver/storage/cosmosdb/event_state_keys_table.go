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
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

// const eventStateKeysSchema = `
// 	CREATE TABLE IF NOT EXISTS roomserver_event_state_keys (
//     event_state_key_nid INTEGER PRIMARY KEY AUTOINCREMENT,
//     event_state_key TEXT NOT NULL UNIQUE
// 	);
// 	INSERT INTO roomserver_event_state_keys (event_state_key_nid, event_state_key)
// 		VALUES (1, '')
// 		ON CONFLICT DO NOTHING;
// `

type EventStateKeysCosmos struct {
	EventStateKeyNID int64  `json:"event_state_key_nid"`
	EventStateKey    string `json:"event_state_key"`
}

type EventStateKeysCosmosData struct {
	Id             string               `json:"id"`
	Pk             string               `json:"_pk"`
	Tn             string               `json:"_sid"`
	Cn             string               `json:"_cn"`
	ETag           string               `json:"_etag"`
	Timestamp      int64                `json:"_ts"`
	EventStateKeys EventStateKeysCosmos `json:"mx_roomserver_event_state_keys"`
}

// Same as insertEventTypeNIDSQL
// const insertEventStateKeyNIDSQL = `
// 	INSERT INTO roomserver_event_state_keys (event_state_key) VALUES ($1)
// 	  ON CONFLICT DO NOTHING;
// `

// 	SELECT event_state_key_nid FROM roomserver_event_state_keys
// 	  WHERE event_state_key = $1
const selectEventStateKeyNIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_roomserver_event_state_keys.event_state_key = @x2"

// // Bulk lookup from string state key to numeric ID for that state key.
// // Takes an array of strings as the query parameter.
// 	SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys
// 	  WHERE event_state_key IN ($1)
const bulkSelectEventStateKeySQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event_state_keys.event_state_key_nid)"

// Bulk lookup from numeric ID to string state key for that state key.
// Takes an array of strings as the query parameter.
// 	SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys
// 	  WHERE event_state_key_nid IN ($1)
const bulkSelectEventStateKeyNIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event_state_keys.event_state_key)"

type eventStateKeyStatements struct {
	db                             *Database
	insertEventStateKeyNIDStmt     string
	selectEventStateKeyNIDStmt     string
	bulkSelectEventStateKeyNIDStmt string
	bulkSelectEventStateKeyStmt    string
	tableName                      string
}

func queryEventStateKeys(s *eventStateKeyStatements, ctx context.Context, qry string, params map[string]interface{}) ([]EventStateKeysCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []EventStateKeysCosmosData

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

func getEventStateKeys(s *eventStateKeyStatements, ctx context.Context, pk string, docId string) (*EventStateKeysCosmosData, error) {
	response := EventStateKeysCosmosData{}
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

func NewCosmosDBEventStateKeysTable(db *Database) (tables.EventStateKeys, error) {
	s := &eventStateKeyStatements{
		db: db,
	}
	// return s, shared.StatementList{
	// 	{&s.insertEventStateKeyNIDStmt, insertEventStateKeyNIDSQL},
	s.selectEventStateKeyNIDStmt = selectEventStateKeyNIDSQL
	s.bulkSelectEventStateKeyNIDStmt = bulkSelectEventStateKeyNIDSQL
	s.bulkSelectEventStateKeyStmt = bulkSelectEventStateKeySQL
	// }.Prepare(db)
	s.tableName = "event_state_keys"
	//Add in the initial data
	ensureEventStateKeys(s, context.Background())
	return s, nil
}

func ensureEventStateKeys(s *eventStateKeyStatements, ctx context.Context) {

	// INSERT INTO roomserver_event_state_keys (event_state_key_nid, event_state_key)
	// VALUES (1, '')
	// ON CONFLICT DO NOTHING;

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     event_state_key TEXT NOT NULL UNIQUE
	docId := ""
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	data := EventStateKeysCosmos{
		EventStateKey:    "",
		EventStateKeyNID: 1,
	}

	//     event_state_key_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	dbData := EventStateKeysCosmosData{
		Id:             cosmosDocId,
		Tn:             s.db.cosmosConfig.TenantName,
		Cn:             dbCollectionName,
		Pk:             pk,
		Timestamp:      time.Now().Unix(),
		EventStateKeys: data,
	}

	insertEventStateKeyCore(s, ctx, dbData)
}

func insertEventStateKeyCore(s *eventStateKeyStatements, ctx context.Context, dbData EventStateKeysCosmosData) error {
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	if err != nil {
		return err
	}

	return nil
}

func (s *eventStateKeyStatements) InsertEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {

	// INSERT INTO roomserver_event_state_keys (event_state_key) VALUES ($1)
	// ON CONFLICT DO NOTHING;
	if len(eventStateKey) == 0 {
		return 0, cosmosdbutil.ErrNoRows
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     event_state_key TEXT NOT NULL UNIQUE
	docId := eventStateKey
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	existing, _ := getEventStateKeys(s, ctx, pk, cosmosDocId)

	var dbData EventStateKeysCosmosData
	if existing == nil {
		//Not exists, we need to create a new one with a SEQ
		eventStateKeyNIDSeq, seqErr := GetNextEventStateKeyNID(s, ctx)
		if seqErr != nil {
			return -1, seqErr
		}

		data := EventStateKeysCosmos{
			EventStateKey:    eventStateKey,
			EventStateKeyNID: eventStateKeyNIDSeq,
		}

		//     event_state_key_nid INTEGER PRIMARY KEY AUTOINCREMENT,
		dbData = EventStateKeysCosmosData{
			Id:             cosmosDocId,
			Tn:             s.db.cosmosConfig.TenantName,
			Cn:             dbCollectionName,
			Pk:             pk,
			Timestamp:      time.Now().Unix(),
			EventStateKeys: data,
		}
	} else {
		dbData.EventStateKeys = existing.EventStateKeys
	}

	err := insertEventStateKeyCore(s, ctx, dbData)

	return types.EventStateKeyNID(dbData.EventStateKeys.EventStateKeyNID), err
}

func (s *eventStateKeyStatements) SelectEventStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {

	// SELECT event_state_key_nid FROM roomserver_event_state_keys
	//   WHERE event_state_key = $1

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventStateKey,
	}

	response, err := queryEventStateKeys(s, ctx, s.selectEventStateKeyNIDStmt, params)

	if err != nil {
		return 0, err
	}
	//See storage.assignStateKeyNID()
	if len(response) == 0 {
		return 0, cosmosdbutil.ErrNoRows
	}

	return types.EventStateKeyNID(response[0].EventStateKeys.EventStateKeyNID), err
}

func (s *eventStateKeyStatements) BulkSelectEventStateKeyNID(
	ctx context.Context, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	iEventStateKeys := make([]interface{}, len(eventStateKeys))
	for k, v := range eventStateKeys {
		iEventStateKeys[k] = v
	}

	// SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys
	//   WHERE event_state_key IN ($1)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventStateKeys,
	}

	response, err := queryEventStateKeys(s, ctx, s.bulkSelectEventStateKeyNIDStmt, params)

	if err != nil {
		return nil, err
	}

	result := make(map[string]types.EventStateKeyNID, len(eventStateKeys))
	for _, item := range response {
		result[item.EventStateKeys.EventStateKey] = types.EventStateKeyNID(item.EventStateKeys.EventStateKeyNID)
	}
	return result, nil
}

func (s *eventStateKeyStatements) BulkSelectEventStateKey(
	ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {

	// SELECT event_state_key, event_state_key_nid FROM roomserver_event_state_keys
	//   WHERE event_state_key_nid IN ($1)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventStateKeyNIDs,
	}

	response, err := queryEventStateKeys(s, ctx, s.bulkSelectEventStateKeyStmt, params)

	if err != nil {
		return nil, err
	}
	result := make(map[types.EventStateKeyNID]string, len(eventStateKeyNIDs))
	for _, item := range response {
		result[types.EventStateKeyNID(item.EventStateKeys.EventStateKeyNID)] = item.EventStateKeys.EventStateKey
	}
	return result, nil
}
