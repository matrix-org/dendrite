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

// const eventTypesSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_event_types (
//     event_type_nid INTEGER PRIMARY KEY AUTOINCREMENT,
//     event_type TEXT NOT NULL UNIQUE
//   );
// INSERT INTO roomserver_event_types (event_type_nid, event_type) VALUES
// (1, 'm.room.create'),
// (2, 'm.room.power_levels'),
// (3, 'm.room.join_rules'),
// (4, 'm.room.third_party_invite'),
// (5, 'm.room.member'),
// (6, 'm.room.redaction'),
// (7, 'm.room.history_visibility') ON CONFLICT DO NOTHING;
// `

type EventTypeCosmosData struct {
	Id        string          `json:"id"`
	Pk        string          `json:"_pk"`
	Cn        string          `json:"_cn"`
	ETag      string          `json:"_etag"`
	Timestamp int64           `json:"_ts"`
	EventType EventTypeCosmos `json:"mx_roomserver_event_type"`
}

type EventTypeCosmos struct {
	EventTypeNID int64  `json:"event_type_nid"`
	EventType    string `json:"event_type"`
}

// Assign a new numeric event type ID.
// The usual case is that the event type is not in the database.
// In that case the ID will be assigned using the next value from the sequence.
// We use `RETURNING` to tell postgres to return the assigned ID.
// But it's possible that the type was added in a query that raced with us.
// This will result in a conflict on the event_type_unique constraint, in this
// case we do nothing. Postgresql won't return a row in that case so we rely on
// the caller catching the sql.ErrNoRows error and running a select to get the row.
// We could get postgresql to return the row on a conflict by updating the row
// but it doesn't seem like a good idea to modify the rows just to make postgresql
// return it. Modifying the rows will cause postgres to assign a new tuple for the
// row even though the data doesn't change resulting in unncesssary modifications
// to the indexes.
// const insertEventTypeNIDSQL = `
// 	INSERT INTO roomserver_event_types (event_type) VALUES ($1)
// 	  ON CONFLICT DO NOTHING;
// `

// const insertEventTypeNIDResultSQL = `
// 	SELECT event_type_nid FROM roomserver_event_types
// 		WHERE rowid = last_insert_rowid();
// `

// const selectEventTypeNIDSQL = `
// 	SELECT event_type_nid FROM roomserver_event_types WHERE event_type = $1
// `

// Bulk lookup from string event type to numeric ID for that event type.
// Takes an array of strings as the query parameter.
// 	SELECT event_type, event_type_nid FROM roomserver_event_types
// 	  WHERE event_type IN ($1)
const bulkSelectEventTypeNIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event_type.event_type)"

type eventTypeStatements struct {
	db *Database
	// insertEventTypeNIDStmt       *sql.Stmt
	// insertEventTypeNIDResultStmt *sql.Stmt
	// selectEventTypeNIDStmt     *sql.Stmt
	bulkSelectEventTypeNIDStmt string
	tableName                  string
}

func NewCosmosDBEventTypesTable(db *Database) (tables.EventTypes, error) {
	s := &eventTypeStatements{
		db: db,
	}

	// 	return s, shared.StatementList{
	// 		{&s.insertEventTypeNIDStmt, insertEventTypeNIDSQL},
	// 		{&s.insertEventTypeNIDResultStmt, insertEventTypeNIDResultSQL},
	// 		{&s.selectEventTypeNIDStmt, selectEventTypeNIDSQL},
	s.bulkSelectEventTypeNIDStmt = bulkSelectEventTypeNIDSQL
	// 	}.Prepare(db)
	s.tableName = "event_types"
	ensureEventTypes(s, context.Background())
	return s, nil
}

func queryEventTypes(s *eventTypeStatements, ctx context.Context, qry string, params map[string]interface{}) ([]EventTypeCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []EventTypeCosmosData

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

func (s *eventTypeStatements) InsertEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
	//We need to create a new one with a SEQ
	eventTypeNIDSeq, seqErr := GetNextEventTypeNID(s, ctx)
	if seqErr != nil {
		return -1, seqErr
	}

	data := EventTypeCosmos{
		EventType:    eventType,
		EventTypeNID: eventTypeNIDSeq,
	}

	dbData, err := insertEventTypeCore(s, ctx, data)

	if err != nil {
		return 0, err
	}

	return types.EventTypeNID(dbData.EventTypeNID), err
}

func insertEventTypeCore(s *eventTypeStatements, ctx context.Context, eventType EventTypeCosmos) (*EventTypeCosmos, error) {
	// INSERT INTO roomserver_event_types (event_type) VALUES ($1)
	//   ON CONFLICT DO NOTHING;
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	//Unique on eventType
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, eventType.EventType)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	var dbData = EventTypeCosmosData{
		Id:        cosmosDocId,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		EventType: eventType,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	if err != nil {
		dbData, errGet := selectEventTypeCore(s, ctx, eventType.EventType)
		if errGet != nil {
			return nil, errGet
		}
		return dbData, nil
	}

	return &dbData.EventType, err
}

func ensureEventTypes(s *eventTypeStatements, ctx context.Context) error {
	// INSERT INTO roomserver_event_types (event_type_nid, event_type) VALUES
	// (1, 'm.room.create'),
	// (2, 'm.room.power_levels'),
	// (3, 'm.room.join_rules'),
	// (4, 'm.room.third_party_invite'),
	// (5, 'm.room.member'),
	// (6, 'm.room.redaction'),
	// (7, 'm.room.history_visibility') ON CONFLICT DO NOTHING;

	// (1, 'm.room.create'),
	_, err := insertEventTypeCore(s, context.Background(), EventTypeCosmos{EventTypeNID: 1, EventType: "m.room.create"})
	if err != nil {
		return err
	}
	// (2, 'm.room.power_levels'),
	_, err = insertEventTypeCore(s, context.Background(), EventTypeCosmos{EventTypeNID: 2, EventType: "m.room.power_levels"})
	if err != nil {
		return err
	}
	// (3, 'm.room.join_rules'),
	_, err = insertEventTypeCore(s, context.Background(), EventTypeCosmos{EventTypeNID: 3, EventType: "m.room.join_rules"})
	if err != nil {
		return err
	}
	// (4, 'm.room.third_party_invite'),
	_, err = insertEventTypeCore(s, context.Background(), EventTypeCosmos{EventTypeNID: 4, EventType: "m.room.third_party_invite"})
	if err != nil {
		return err
	}
	// (5, 'm.room.member'),
	_, err = insertEventTypeCore(s, context.Background(), EventTypeCosmos{EventTypeNID: 5, EventType: "m.room.member"})
	if err != nil {
		return err
	}
	// (6, 'm.room.redaction'),
	_, err = insertEventTypeCore(s, context.Background(), EventTypeCosmos{EventTypeNID: 6, EventType: "m.room.redaction"})
	if err != nil {
		return err
	}
	// (7, 'm.room.history_visibility') ON CONFLICT DO NOTHING;
	_, err = insertEventTypeCore(s, context.Background(), EventTypeCosmos{EventTypeNID: 7, EventType: "m.room.history_visibility"})
	if err != nil {
		return err
	}
	return nil
}

func selectEventTypeCore(s *eventTypeStatements, ctx context.Context, eventType string) (*EventTypeCosmos, error) {
	var response EventTypeCosmosData
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, eventType)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		cosmosDocId,
		&response)

	if err != nil {
		return nil, err
	}

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response.EventType, nil
}

func (s *eventTypeStatements) SelectEventTypeNID(
	ctx context.Context, tx *sql.Tx, eventType string,
) (types.EventTypeNID, error) {

	// SELECT event_type_nid FROM roomserver_event_types WHERE event_type = $1

	dbData, err := selectEventTypeCore(s, ctx, eventType)
	if err != nil {
		return -1, err
	}
	return types.EventTypeNID(dbData.EventTypeNID), nil
}

func (s *eventTypeStatements) BulkSelectEventTypeNID(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {

	// SELECT event_type, event_type_nid FROM roomserver_event_types
	// WHERE event_type IN ($1)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventTypes,
	}

	response, err := queryEventTypes(s, ctx, s.bulkSelectEventTypeNIDStmt, params)

	if err != nil {
		return nil, err
	}

	result := make(map[string]types.EventTypeNID, len(eventTypes))
	for _, item := range response {
		var eventType string
		var eventTypeNID int64
		eventType = item.EventType.EventType
		eventTypeNID = item.EventType.EventTypeNID
		result[eventType] = types.EventTypeNID(eventTypeNID)
	}
	return result, nil
}
