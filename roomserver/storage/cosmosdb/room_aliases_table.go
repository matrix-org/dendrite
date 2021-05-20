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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

// const roomAliasesSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_room_aliases (
//     alias TEXT NOT NULL PRIMARY KEY,
//     room_id TEXT NOT NULL,
//     creator_id TEXT NOT NULL
//   );

//   CREATE INDEX IF NOT EXISTS roomserver_room_id_idx ON roomserver_room_aliases(room_id);
// `

type RoomAliasCosmos struct {
	Alias     string `json:"alias"`
	RoomID    string `json:"room_id"`
	CreatorID string `json:"creator_id"`
}

type RoomAliasCosmosData struct {
	Id        string          `json:"id"`
	Pk        string          `json:"_pk"`
	Cn        string          `json:"_cn"`
	ETag      string          `json:"_etag"`
	Timestamp int64           `json:"_ts"`
	RoomAlias RoomAliasCosmos `json:"mx_roomserver_room_alias"`
}

// const insertRoomAliasSQL = `
// 	INSERT INTO roomserver_room_aliases (alias, room_id, creator_id) VALUES ($1, $2, $3)
// `

// const selectRoomIDFromAliasSQL = `
// SELECT room_id FROM roomserver_room_aliases WHERE alias = $1
// `

// SELECT alias FROM roomserver_room_aliases WHERE room_id = $1
const selectAliasesFromRoomIDSQL = `
	select * from c where c._cn = @x1 and c.mx_roomserver_room_alias.room_id = @x2
`

// const selectCreatorIDFromAliasSQL = `
// 	SELECT creator_id FROM roomserver_room_aliases WHERE alias = $1
// `

// const deleteRoomAliasSQL = `
// 	DELETE FROM roomserver_room_aliases WHERE alias = $1
// `

type roomAliasesStatements struct {
	db *Database
	// insertRoomAliasStmt          *sql.Stmt
	// selectRoomIDFromAliasStmt    string
	selectAliasesFromRoomIDStmt string
	// selectCreatorIDFromAliasStmt string
	// deleteRoomAliasStmt          *sql.Stmt
	tableName string
}

func queryRoomAlias(s *roomAliasesStatements, ctx context.Context, qry string, params map[string]interface{}) ([]RoomAliasCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []RoomAliasCosmosData

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

func getRoomAlias(s *roomAliasesStatements, ctx context.Context, pk string, docId string) (*RoomAliasCosmosData, error) {
	response := RoomAliasCosmosData{}
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

func NewCosmosDBRoomAliasesTable(db *Database) (tables.RoomAliases, error) {
	s := &roomAliasesStatements{
		db: db,
	}
	// _, err := db.Exec(roomAliasesSchema)
	// if err != nil {
	// 	return nil, err
	// }
	// return s, shared.StatementList{
	// 	{&s.insertRoomAliasStmt, insertRoomAliasSQL},
	// 	{&s.selectRoomIDFromAliasStmt, selectRoomIDFromAliasSQL},
	s.selectAliasesFromRoomIDStmt = selectAliasesFromRoomIDSQL
	// 	{&s.selectCreatorIDFromAliasStmt, selectCreatorIDFromAliasSQL},
	// 	{&s.deleteRoomAliasStmt, deleteRoomAliasSQL},
	// }.Prepare(db)
	s.tableName = "room_aliases"
	return s, nil
}

func (s *roomAliasesStatements) InsertRoomAlias(
	ctx context.Context, txn *sql.Tx, alias string, roomID string, creatorUserID string,
) error {

	// 	INSERT INTO roomserver_room_aliases (alias, room_id, creator_id) VALUES ($1, $2, $3)
	data := RoomAliasCosmos{
		Alias:     alias,
		CreatorID: creatorUserID,
		RoomID:    roomID,
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	//     alias TEXT NOT NULL PRIMARY KEY,
	docId := alias
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	var dbData = RoomAliasCosmosData{
		Id:        cosmosDocId,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		RoomAlias: data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
}

func (s *roomAliasesStatements) SelectRoomIDFromAlias(
	ctx context.Context, alias string,
) (roomID string, err error) {

	// SELECT room_id FROM roomserver_room_aliases WHERE alias = $1

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	//     alias TEXT NOT NULL PRIMARY KEY,
	docId := alias
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	response, err := getRoomAlias(s, ctx, pk, cosmosDocId)

	if err != nil {
		return "", err
	}

	if response == nil {
		return "", nil
	}
	roomID = response.RoomAlias.RoomID
	return
}

func (s *roomAliasesStatements) SelectAliasesFromRoomID(
	ctx context.Context, roomID string,
) (aliases []string, err error) {
	aliases = []string{}

	// SELECT alias FROM roomserver_room_aliases WHERE room_id = $1

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	response, err := queryRoomAlias(s, ctx, s.selectAliasesFromRoomIDStmt, params)

	if err != nil {
		return nil, err
	}

	for _, item := range response {
		aliases = append(aliases, item.RoomAlias.Alias)
	}

	return
}

func (s *roomAliasesStatements) SelectCreatorIDFromAlias(
	ctx context.Context, alias string,
) (creatorID string, err error) {

	// 	SELECT creator_id FROM roomserver_room_aliases WHERE alias = $1

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	//     alias TEXT NOT NULL PRIMARY KEY,
	docId := alias
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	response, err := getRoomAlias(s, ctx, pk, cosmosDocId)

	if err != nil {
		return "", err
	}

	if response == nil {
		return "", nil
	}
	creatorID = response.RoomAlias.CreatorID
	return
}

func (s *roomAliasesStatements) DeleteRoomAlias(
	ctx context.Context, txn *sql.Tx, alias string,
) error {

	// 	DELETE FROM roomserver_room_aliases WHERE alias = $1

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	docId := alias
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var options = cosmosdbapi.GetDeleteDocumentOptions(pk)
	var _, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		cosmosDocId,
		options)

	if err != nil {
		return err
	}
	return err
}
