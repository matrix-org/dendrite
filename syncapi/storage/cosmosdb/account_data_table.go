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
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// const accountDataSchema = `
// CREATE TABLE IF NOT EXISTS syncapi_account_data_type (
//     id INTEGER PRIMARY KEY,
//     user_id TEXT NOT NULL,
//     room_id TEXT NOT NULL,
//     type TEXT NOT NULL,
//     UNIQUE (user_id, room_id, type)
// );
// `

type AccountDataTypeCosmos struct {
	ID       int64  `json:"id"`
	UserID   string `json:"user_id"`
	RoomID   string `json:"room_id"`
	DataType string `json:"type"`
}

type AccountDataTypeNumberCosmosData struct {
	Number int64 `json:"number"`
}

type AccountDataTypeCosmosData struct {
	Id              string                `json:"id"`
	Pk              string                `json:"_pk"`
	Cn              string                `json:"_cn"`
	ETag            string                `json:"_etag"`
	Timestamp       int64                 `json:"_ts"`
	AccountDataType AccountDataTypeCosmos `json:"mx_syncapi_account_data_type"`
}

// const insertAccountDataSQL = "" +
// 	"INSERT INTO syncapi_account_data_type (id, user_id, room_id, type) VALUES ($1, $2, $3, $4)" +
// 	" ON CONFLICT (user_id, room_id, type) DO UPDATE" +
// 	" SET id = $5"

// "SELECT room_id, type FROM syncapi_account_data_type" +
// " WHERE user_id = $1 AND id > $2 AND id <= $3" +
// " ORDER BY id ASC"
const selectAccountDataInRangeSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_account_data_type.user_id = @x2 " +
	"and c.mx_syncapi_account_data_type.id > @x3 " +
	"and c.mx_syncapi_account_data_type.id < @x4 " +
	"order by c.mx_syncapi_account_data_type.id "

// "SELECT MAX(id) FROM syncapi_account_data_type"
const selectMaxAccountDataIDSQL = "" +
	"select max(c.mx_syncapi_account_data_type.id) as number from c where c._cn = @x1 "

type accountDataStatements struct {
	db                           *SyncServerDatasource
	streamIDStatements           *streamIDStatements
	insertAccountDataStmt        *sql.Stmt
	selectMaxAccountDataIDStmt   string
	selectAccountDataInRangeStmt string
	tableName                    string
}

func queryAccountDataType(s *accountDataStatements, ctx context.Context, qry string, params map[string]interface{}) ([]AccountDataTypeCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []AccountDataTypeCosmosData

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

func queryAccountDataTypeNumber(s *accountDataStatements, ctx context.Context, qry string, params map[string]interface{}) ([]AccountDataTypeNumberCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []AccountDataTypeNumberCosmosData

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
		return nil, cosmosdbutil.ErrNoRows
	}
	return response, nil
}

func NewCosmosDBAccountDataTable(db *SyncServerDatasource, streamID *streamIDStatements) (tables.AccountData, error) {
	s := &accountDataStatements{
		db:                 db,
		streamIDStatements: streamID,
	}

	s.selectMaxAccountDataIDStmt = selectMaxAccountDataIDSQL
	s.selectAccountDataInRangeStmt = selectAccountDataInRangeSQL
	s.tableName = "account_data_types"
	return s, nil
}

func (s *accountDataStatements) InsertAccountData(
	ctx context.Context, txn *sql.Tx,
	userID, roomID, dataType string,
) (pos types.StreamPosition, err error) {

	// "INSERT INTO syncapi_account_data_type (id, user_id, room_id, type) VALUES ($1, $2, $3, $4)" +
	// " ON CONFLICT (user_id, room_id, type) DO UPDATE" +
	// " SET id = $5"

	pos, err = s.streamIDStatements.nextAccountDataID(ctx, txn)
	if err != nil {
		return
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     UNIQUE (user_id, room_id, type)
	docId := fmt.Sprintf("%s_%s_%s", userID, roomID, dataType)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	data := AccountDataTypeCosmos{
		ID:       int64(pos),
		UserID:   userID,
		RoomID:   roomID,
		DataType: dataType,
	}

	dbData := &AccountDataTypeCosmosData{
		Id:              cosmosDocId,
		Cn:              dbCollectionName,
		Pk:              pk,
		Timestamp:       time.Now().Unix(),
		AccountDataType: data,
	}

	// _, err = sqlutil.TxStmt(txn, s.insertAccountDataStmt).ExecContext(ctx, pos, userID, roomID, dataType, pos)
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return
}

func (s *accountDataStatements) SelectAccountDataInRange(
	ctx context.Context,
	userID string,
	r types.Range,
	accountDataFilterPart *gomatrixserverlib.EventFilter,
) (data map[string][]string, err error) {
	data = make(map[string][]string)

	// "SELECT room_id, type FROM syncapi_account_data_type" +
	// " WHERE user_id = $1 AND id > $2 AND id <= $3" +
	// " ORDER BY id ASC"

	// rows, err := s.selectAccountDataInRangeStmt.QueryContext(ctx, userID, r.Low(), r.High())
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
		"@x3": r.Low(),
		"@x4": r.High(),
	}

	rows, err := queryAccountDataType(s, ctx, s.selectAccountDataInRangeStmt, params)

	if err != nil {
		return
	}

	var entries int

	for _, item := range rows {
		var dataType string
		var roomID string
		roomID = item.AccountDataType.RoomID
		dataType = item.AccountDataType.DataType

		// check if we should add this by looking at the filter.
		// It would be nice if we could do this in SQL-land, but the mix of variadic
		// and positional parameters makes the query annoyingly hard to do, it's easier
		// and clearer to do it in Go-land. If there are no filters for [not]types then
		// this gets skipped.
		for _, includeType := range accountDataFilterPart.Types {
			if includeType != dataType { // TODO: wildcard support
				continue
			}
		}
		for _, excludeType := range accountDataFilterPart.NotTypes {
			if excludeType == dataType { // TODO: wildcard support
				continue
			}
		}

		if len(data[roomID]) > 0 {
			data[roomID] = append(data[roomID], dataType)
		} else {
			data[roomID] = []string{dataType}
		}
		entries++
		if entries >= accountDataFilterPart.Limit {
			break
		}
	}

	return data, nil
}

func (s *accountDataStatements) SelectMaxAccountDataID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {

	// "SELECT MAX(id) FROM syncapi_account_data_type"

	var nullableID sql.NullInt64
	// err = sqlutil.TxStmt(txn, s.selectMaxAccountDataIDStmt).QueryRowContext(ctx).Scan(&nullableID)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	rows, err := queryAccountDataTypeNumber(s, ctx, s.selectMaxAccountDataIDStmt, params)

	if err != cosmosdbutil.ErrNoRows && len(rows) == 1 {
		nullableID.Int64 = rows[0].Number
	}

	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
