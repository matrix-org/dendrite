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

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

// const eventJSONSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_event_json (
//     event_nid INTEGER NOT NULL PRIMARY KEY,
//     event_json TEXT NOT NULL
//   );
// `

type eventJSONCosmos struct {
	EventNID  int64  `json:"event_nid"`
	EventJSON []byte `json:"event_json"`
}

type eventJSONCosmosData struct {
	cosmosdbapi.CosmosDocument
	EventJSON eventJSONCosmos `json:"mx_roomserver_event_json"`
}

// const insertEventJSONSQL = `
// 	INSERT OR REPLACE INTO roomserver_event_json (event_nid, event_json) VALUES ($1, $2)
// `

// Bulk event JSON lookup by numeric event ID.
// Sort by the numeric event ID.
// This means that we can use binary search to lookup by numeric event ID.
// 	SELECT event_nid, event_json FROM roomserver_event_json
// 	  WHERE event_nid IN ($1)
// 	  ORDER BY event_nid ASC
const bulkSelectEventJSONSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event_json.event_nid) " +
	"order by c.mx_roomserver_event_json.event_nid asc"

type eventJSONStatements struct {
	db *Database
	// insertEventJSONStmt     *sql.Stmt
	bulkSelectEventJSONStmt string
	tableName               string
}

func (s *eventJSONStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *eventJSONStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getEventJSON(s *eventJSONStatements, ctx context.Context, pk string, docId string) (*eventJSONCosmosData, error) {
	response := eventJSONCosmosData{}
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

func NewCosmosDBEventJSONTable(db *Database) (tables.EventJSON, error) {
	s := &eventJSONStatements{
		db: db,
	}
	// _, err := db.Exec(eventJSONSchema)
	// if err != nil {
	// 	return nil, err
	// }
	// return s, shared.StatementList{
	// 	{&s.insertEventJSONStmt, insertEventJSONSQL},
	s.bulkSelectEventJSONStmt = bulkSelectEventJSONSQL
	// }.Prepare(db)
	s.tableName = "event_json"
	return s, nil
}

func (s *eventJSONStatements) InsertEventJSON(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, eventJSON []byte,
) error {

	// _, err := sqlutil.TxStmt(txn, s.insertEventJSONStmt).ExecContext(ctx, int64(eventNID), eventJSON)
	// INSERT OR REPLACE INTO roomserver_event_json (event_nid, event_json) VALUES ($1, $2)

	docId := fmt.Sprintf("%d", eventNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getEventJSON(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.EventJSON.EventJSON = eventJSON
	} else {
		data := eventJSONCosmos{
			EventNID:  int64(eventNID),
			EventJSON: eventJSON,
		}

		dbData = &eventJSONCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			EventJSON:      data,
		}
	}

	//Insert OR Replace
	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func (s *eventJSONStatements) BulkSelectEventJSON(
	ctx context.Context, eventNIDs []types.EventNID,
) ([]tables.EventJSONPair, error) {

	// SELECT event_nid, event_json FROM roomserver_event_json
	//   WHERE event_nid IN ($1)
	//   ORDER BY event_nid ASC

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNIDs,
	}

	var rows []eventJSONCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectEventJSONStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	// We know that we will only get as many results as event NIDs
	// because of the unique constraint on event NIDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than NIDs so we adjust the length of the slice before returning it.
	results := make([]tables.EventJSONPair, len(eventNIDs))
	i := 0
	for _, item := range rows {
		result := &results[i]
		result.EventNID = types.EventNID(item.EventJSON.EventNID)
		result.EventJSON = item.EventJSON.EventJSON
		i++
	}
	return results[:i], nil
}
