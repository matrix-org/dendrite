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
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

// var staleDeviceListsSchema = `
// -- Stores whether a user's device lists are stale or not.
// CREATE TABLE IF NOT EXISTS keyserver_stale_device_lists (
//     user_id TEXT PRIMARY KEY NOT NULL,
// 	domain TEXT NOT NULL,
// 	is_stale BOOLEAN NOT NULL,
// 	ts_added_secs BIGINT NOT NULL
// );

// CREATE INDEX IF NOT EXISTS keyserver_stale_device_lists_idx ON keyserver_stale_device_lists (domain, is_stale);
// `

type StaleDeviceListCosmos struct {
	UserID  string `json:"user_id"`
	Domain  string `json:"domain"`
	IsStale bool   `json:"is_stale"`
	// Use the CosmosDB.Timestamp for this one
	// ts_added_secs    int64  `json:"ts_added_secs"`
}

type StaleDeviceListCosmosData struct {
	Id              string                `json:"id"`
	Pk              string                `json:"_pk"`
	Tn              string                `json:"_sid"`
	Cn              string                `json:"_cn"`
	ETag            string                `json:"_etag"`
	Timestamp       int64                 `json:"_ts"`
	StaleDeviceList StaleDeviceListCosmos `json:"mx_keyserver_stale_device_list"`
}

// const upsertStaleDeviceListSQL = "" +
// 	"INSERT INTO keyserver_stale_device_lists (user_id, domain, is_stale, ts_added_secs)" +
// 	" VALUES ($1, $2, $3, $4)" +
// 	" ON CONFLICT (user_id)" +
// 	" DO UPDATE SET is_stale = $3, ts_added_secs = $4"

// "SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1 AND domain = $2"
const selectStaleDeviceListsWithDomainsSQL = "" +
	"select * from c where c._sid = @x1 and c._cn = @x2 " +
	"and c.mx_keyserver_stale_device_list.is_stale = @x3 " +
	"and c.mx_keyserver_stale_device_list.domain = @x4 "

// "SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1"
const selectStaleDeviceListsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_stale_device_list.is_stale = @x2 "

type staleDeviceListsStatements struct {
	db *Database
	// upsertStaleDeviceListStmt             *sql.Stmt
	selectStaleDeviceListsWithDomainsStmt string
	selectStaleDeviceListsStmt            string
	tableName                             string
}

func queryStaleDeviceList(s *staleDeviceListsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]StaleDeviceListCosmosData, error) {
	var response []StaleDeviceListCosmosData

	var optionsQry = cosmosdbapi.GetQueryAllPartitionsDocumentsOptions()
	var query = cosmosdbapi.GetQuery(qry, params)
	var _, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
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

func NewCosmosDBStaleDeviceListsTable(db *Database) (tables.StaleDeviceLists, error) {
	s := &staleDeviceListsStatements{
		db: db,
	}
	s.selectStaleDeviceListsStmt = selectStaleDeviceListsSQL
	s.selectStaleDeviceListsWithDomainsStmt = selectStaleDeviceListsWithDomainsSQL
	s.tableName = "stale_device_lists"
	return s, nil
}

func (s *staleDeviceListsStatements) InsertStaleDeviceList(ctx context.Context, userID string, isStale bool) error {

	// "INSERT INTO keyserver_stale_device_lists (user_id, domain, is_stale, ts_added_secs)" +
	// " VALUES ($1, $2, $3, $4)" +
	// " ON CONFLICT (user_id)" +
	// " DO UPDATE SET is_stale = $3, ts_added_secs = $4"

	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return err
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	//     user_id TEXT PRIMARY KEY NOT NULL,
	docId := userID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)

	data := StaleDeviceListCosmos{
		Domain:  string(domain),
		IsStale: isStale,
		UserID:  userID,
	}

	dbData := StaleDeviceListCosmosData{
		Id:              cosmosDocId,
		Tn:              s.db.cosmosConfig.TenantName,
		Cn:              dbCollectionName,
		Pk:              pk,
		Timestamp:       time.Now().Unix(),
		StaleDeviceList: data,
	}

	// _, err := s.upsertKeyChangeStmt.ExecContext(ctx, partition, offset, userID)
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	return err
}

func (s *staleDeviceListsStatements) SelectUserIDsWithStaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error) {
	// we only query for 1 domain or all domains so optimise for those use cases
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	if len(domains) == 0 {

		// "SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1"
		// rows, err := s.selectStaleDeviceListsStmt.QueryContext(ctx, true)
		params := map[string]interface{}{
			"@x1": s.db.cosmosConfig.TenantName,
			"@x2": dbCollectionName,
			"@x3": true,
		}
		rows, err := queryStaleDeviceList(s, ctx, s.selectStaleDeviceListsWithDomainsStmt, params)

		if err != nil {
			return nil, err
		}
		return rowsToUserIDs(ctx, rows)
	}
	var result []string
	for _, domain := range domains {

		// "SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1 AND domain = $2"
		// rows, err := s.selectStaleDeviceListsWithDomainsStmt.QueryContext(ctx, true, string(domain))
		params := map[string]interface{}{
			"@x1": dbCollectionName,
			"@x2": true,
			"@x3": string(domain),
		}

		rows, err := queryStaleDeviceList(s, ctx, s.selectStaleDeviceListsWithDomainsStmt, params)

		if err != nil {
			return nil, err
		}
		userIDs, err := rowsToUserIDs(ctx, rows)
		if err != nil {
			return nil, err
		}
		result = append(result, userIDs...)
	}
	return result, nil
}

func rowsToUserIDs(ctx context.Context, rows []StaleDeviceListCosmosData) (result []string, err error) {
	for _, item := range rows {
		var userID string
		userID = item.StaleDeviceList.UserID
		result = append(result, userID)
	}
	return result, nil
}
