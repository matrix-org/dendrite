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
	"time"

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

type AccountDataCosmosData struct {
	Id          string            `json:"id"`
	Pk          string            `json:"_pk"`
	Cn          string            `json:"_cn"`
	ETag        string            `json:"_etag"`
	Timestamp   int64             `json:"_ts"`
	AccountData AccountDataCosmos `json:"mx_userapi_accountdata"`
}

type AccountDataCosmos struct {
	LocalPart string `json:"local_part"`
	RoomId    string `json:"room_id"`
	Type      string `json:"type"`
	Content   []byte `json:"content"`
}

type accountDataStatements struct {
	db *Database
	// insertAccountDataStmt       *sql.Stmt
	selectAccountDataStmt       string
	selectAccountDataByTypeStmt string
	tableName                   string
}

func (s *accountDataStatements) prepare(db *Database) (err error) {
	s.db = db
	s.selectAccountDataStmt = "select * from c where c._cn = @x1 and c.mx_userapi_accountdata.local_part = @x2"
	s.selectAccountDataByTypeStmt = "select * from c where c._cn = @x1 and c.mx_userapi_accountdata.local_part = @x2 and c.mx_userapi_accountdata.room_id = @x3 and c.mx_userapi_accountdata.type = @x4"
	s.tableName = "account_data"
	return
}

func (s *accountDataStatements) insertAccountData(
	ctx context.Context, localpart, roomID, dataType string, content json.RawMessage,
) error {

	// 	INSERT INTO account_data(localpart, room_id, type, content) VALUES($1, $2, $3, $4)
	// 	ON CONFLICT (localpart, room_id, type) DO UPDATE SET content = $4
	var result = AccountDataCosmos{
		LocalPart: localpart,
		RoomId:    roomID,
		Type:      dataType,
		Content:   content,
	}

	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accountDatas.tableName)
	id := ""
	if roomID == "" {
		id = fmt.Sprintf("%s_%s", result.LocalPart, result.Type)
	} else {
		id = fmt.Sprintf("%s_%s_%s", result.LocalPart, result.RoomId, result.Type)
	}

	var dbData = AccountDataCosmosData{
		Id:          cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, id),
		Cn:          dbCollectionName,
		Pk:          cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName),
		Timestamp:   time.Now().Unix(),
		AccountData: result,
	}

	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
		dbData,
		options)

	return err
}

func (s *accountDataStatements) selectAccountData(
	ctx context.Context, localpart string,
) (
	/* global */ map[string]json.RawMessage,
	/* rooms */ map[string]map[string]json.RawMessage,
	error,
) {
	// 	"SELECT room_id, type, content FROM account_data WHERE localpart = $1"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accountDatas.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []AccountDataCosmosData{}
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectAccountDataStmt, params)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		config.DatabaseName,
		config.TenantName,
		query,
		&response,
		options)

	if ex != nil {
		return nil, nil, ex
	}

	global := map[string]json.RawMessage{}
	rooms := map[string]map[string]json.RawMessage{}

	for i := 0; i < len(response); i++ {
		var row = response[i]
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
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accountDatas.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []AccountDataCosmosData{}
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
		"@x3": roomID,
		"@x4": dataType,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(s.selectAccountDataByTypeStmt, params)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		config.DatabaseName,
		config.TenantName,
		query,
		&response,
		options)

	if ex != nil {
		return nil, ex
	}

	if len(response) == 0 {
		return data, nil
	}

	bytes = response[0].AccountData.Content

	data = json.RawMessage(bytes)
	return
}
