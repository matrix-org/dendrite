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
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

// const publishedSchema = `
// -- Stores which rooms are published in the room directory
// CREATE TABLE IF NOT EXISTS roomserver_published (
//     -- The room ID of the room
//     room_id TEXT NOT NULL PRIMARY KEY,
//     -- Whether it is published or not
//     published BOOLEAN NOT NULL DEFAULT false
// );
// `

type PublishCosmos struct {
	RoomID    string `json:"room_id"`
	Published bool   `json:"published"`
}

type PublishCosmosData struct {
	Id        string        `json:"id"`
	Pk        string        `json:"_pk"`
	Cn        string        `json:"_cn"`
	ETag      string        `json:"_etag"`
	Timestamp int64         `json:"_ts"`
	Publish   PublishCosmos `json:"mx_roomserver_publish"`
}

// const upsertPublishedSQL = "" +
// 	"INSERT OR REPLACE INTO roomserver_published (room_id, published) VALUES ($1, $2)"

// "SELECT room_id FROM roomserver_published WHERE published = $1 ORDER BY room_id ASC"
const selectAllPublishedSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_publish.published = @x2" +
	" order by c.mx_roomserver_publish.room_id asc"

// const selectPublishedSQL = "" +
// 	"SELECT published FROM roomserver_published WHERE room_id = $1"

type publishedStatements struct {
	db *Database
	// upsertPublishedStmt    *sql.Stmt
	selectAllPublishedStmt string
	// selectPublishedStmt    *sql.Stmt
	tableName string
}

func queryPublish(s *publishedStatements, ctx context.Context, qry string, params map[string]interface{}) ([]PublishCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []PublishCosmosData

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

func getPublish(s *publishedStatements, ctx context.Context, pk string, docId string) (*PublishCosmosData, error) {
	response := PublishCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response, err
}

func NewCosmosDBPublishedTable(db *Database) (tables.Published, error) {
	s := &publishedStatements{
		db: db,
	}
	// _, err := db.Exec(publishedSchema)
	// if err != nil {
	// 	return nil, err
	// }
	// return s, shared.StatementList{
	// 	{&s.upsertPublishedStmt, upsertPublishedSQL},
	s.selectAllPublishedStmt = selectAllPublishedSQL
	// 	{&s.selectPublishedStmt, selectPublishedSQL},
	// }.Prepare(db)
	s.tableName = "published"
	return s, nil
}

func (s *publishedStatements) UpsertRoomPublished(
	ctx context.Context, txn *sql.Tx, roomID string, published bool,
) error {

	// 	"INSERT OR REPLACE INTO roomserver_published (room_id, published) VALUES ($1, $2)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     room_id TEXT NOT NULL PRIMARY KEY,
	docId := roomID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	data := PublishCosmos{
		RoomID:    roomID,
		Published: false,
	}

	var dbData = PublishCosmosData{
		Id:        cosmosDocId,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		Publish:   data,
	}

	// 	"INSERT OR REPLACE INTO roomserver_published (room_id, published) VALUES ($1, $2)"
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
}

func (s *publishedStatements) SelectPublishedFromRoomID(
	ctx context.Context, roomID string,
) (published bool, err error) {

	// 	"SELECT published FROM roomserver_published WHERE room_id = $1"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     room_id TEXT NOT NULL PRIMARY KEY,
	docId := roomID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	response, err := getPublish(s, ctx, pk, cosmosDocId)
	if err != nil {
		return false, err
	}
	return response.Publish.Published, nil
}

func (s *publishedStatements) SelectAllPublishedRooms(
	ctx context.Context, published bool,
) ([]string, error) {

	// "SELECT room_id FROM roomserver_published WHERE published = $1 ORDER BY room_id ASC"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": published,
	}

	response, err := queryPublish(s, ctx, s.selectAllPublishedStmt, params)
	if err != nil {
		return nil, err
	}

	var roomIDs []string
	for _, item := range response {
		roomIDs = append(roomIDs, item.Publish.RoomID)
	}
	return roomIDs, nil
}
