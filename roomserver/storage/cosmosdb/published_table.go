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

type publishCosmos struct {
	RoomID    string `json:"room_id"`
	Published bool   `json:"published"`
}

type publishCosmosData struct {
	cosmosdbapi.CosmosDocument
	Publish publishCosmos `json:"mx_roomserver_publish"`
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

func (s *publishedStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *publishedStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getPublish(s *publishedStatements, ctx context.Context, pk string, docId string) (*publishCosmosData, error) {
	response := publishCosmosData{}
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

	//     room_id TEXT NOT NULL PRIMARY KEY,
	docId := roomID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	dbData, _ := getPublish(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.Publish.Published = published
	} else {
		data := publishCosmos{
			RoomID:    roomID,
			Published: false,
		}

		dbData = &publishCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			Publish:        data,
		}
	}

	// 	"INSERT OR REPLACE INTO roomserver_published (room_id, published) VALUES ($1, $2)"
	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func (s *publishedStatements) SelectPublishedFromRoomID(
	ctx context.Context, roomID string,
) (published bool, err error) {

	// 	"SELECT published FROM roomserver_published WHERE room_id = $1"
	//     room_id TEXT NOT NULL PRIMARY KEY,
	docId := roomID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	response, err := getPublish(s, ctx, s.getPartitionKey(), cosmosDocId)
	if err != nil {
		return false, err
	}
	return response.Publish.Published, nil
}

func (s *publishedStatements) SelectAllPublishedRooms(
	ctx context.Context, published bool,
) ([]string, error) {

	// "SELECT room_id FROM roomserver_published WHERE published = $1 ORDER BY room_id ASC"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": published,
	}

	var rows []publishCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectAllPublishedStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	var roomIDs []string
	for _, item := range rows {
		roomIDs = append(roomIDs, item.Publish.RoomID)
	}
	return roomIDs, nil
}
