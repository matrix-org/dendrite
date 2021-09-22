// Copyright 2017 Vector Creations Ltd
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
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
)

// const accountDataSchema = `
// -- Stores data about accounts data.
// CREATE TABLE IF NOT EXISTS account_data (
//     -- The Matrix user ID localpart for this account
//     localpart TEXT NOT NULL,
//     -- The room ID for this data (empty string if not specific to a room)
//     room_id TEXT,
//     -- The account data type
//     type TEXT NOT NULL,
//     -- The account data content
//     content TEXT NOT NULL,

//     PRIMARY KEY(localpart, room_id, type)
// );
// `

type accountDataCosmos struct {
	LocalPart string `json:"local_part"`
	RoomId    string `json:"room_id"`
	Type      string `json:"type"`
	Content   []byte `json:"content"`
}

type accountDataCosmosData struct {
	cosmosdbapi.CosmosDocument
	AccountData accountDataCosmos `json:"mx_userapi_accountdata"`
}

type accountDataStatements struct {
	db *Database
	// insertAccountDataStmt       *sql.Stmt
	selectAccountDataStmt       string
	selectAccountDataByTypeStmt string
	tableName                   string
}

func (s *accountDataStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *accountDataStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func (s *accountDataStatements) prepare(db *Database) (err error) {
	s.db = db
	s.selectAccountDataStmt = "select * from c where c._cn = @x1 and c.mx_userapi_accountdata.local_part = @x2"
	s.selectAccountDataByTypeStmt = "select * from c where c._cn = @x1 and c.mx_userapi_accountdata.local_part = @x2 and c.mx_userapi_accountdata.room_id = @x3 and c.mx_userapi_accountdata.type = @x4"
	s.tableName = "account_data"
	return
}

func getAccountData(s *accountDataStatements, ctx context.Context, pk string, docId string) (*accountDataCosmosData, error) {
	response := accountDataCosmosData{}
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

func (s *accountDataStatements) insertAccountData(
	ctx context.Context, localpart, roomID, dataType string, content json.RawMessage,
) error {

	// 	INSERT INTO account_data(localpart, room_id, type, content) VALUES($1, $2, $3, $4)
	// 	ON CONFLICT (localpart, room_id, type) DO UPDATE SET content = $4
	id := ""
	if roomID == "" {
		id = fmt.Sprintf("%s_%s", localpart, dataType)
	} else {
		id = fmt.Sprintf("%s_%s_%s", localpart, roomID, dataType)
	}

	docId := id
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getAccountData(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		// 	ON CONFLICT (localpart, room_id, type) DO UPDATE SET content = $4
		dbData.SetUpdateTime()
		dbData.AccountData.Content = content
	} else {
		var result = accountDataCosmos{
			LocalPart: localpart,
			RoomId:    roomID,
			Type:      dataType,
			Content:   content,
		}

		dbData = &accountDataCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			AccountData:    result,
		}
	}

	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func (s *accountDataStatements) selectAccountData(
	ctx context.Context, localpart string,
) (
	/* global */ map[string]json.RawMessage,
	/* rooms */ map[string]map[string]json.RawMessage,
	error,
) {
	// 	"SELECT room_id, type, content FROM account_data WHERE localpart = $1"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
	}
	var rows []accountDataCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectAccountDataStmt, params, &rows)

	if err != nil {
		return nil, nil, err
	}

	global := map[string]json.RawMessage{}
	rooms := map[string]map[string]json.RawMessage{}

	for i := 0; i < len(rows); i++ {
		var row = rows[i]
		var roomID = row.AccountData.RoomId
		if roomID != "" {
			if _, ok := rooms[row.AccountData.RoomId]; !ok {
				rooms[roomID] = map[string]json.RawMessage{}
			}
			rooms[roomID][row.AccountData.Type] = row.AccountData.Content
		} else {
			global[row.AccountData.Type] = row.AccountData.Content
		}
	}

	return global, rooms, nil
}

func (s *accountDataStatements) selectAccountDataByType(
	ctx context.Context, localpart, roomID, dataType string,
) (data json.RawMessage, err error) {
	var bytes []byte

	// 	"SELECT content FROM account_data WHERE localpart = $1 AND room_id = $2 AND type = $3"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": localpart,
		"@x3": roomID,
		"@x4": dataType,
	}
	var rows []accountDataCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectAccountDataByTypeStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return data, nil
	}

	bytes = rows[0].AccountData.Content

	data = json.RawMessage(bytes)
	return
}
