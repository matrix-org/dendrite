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

type staleDeviceListCosmos struct {
	UserID    string `json:"user_id"`
	Domain    string `json:"domain"`
	IsStale   bool   `json:"is_stale"`
	AddedSecs int64  `json:"ts_added_secs"`
}

type staleDeviceListCosmosData struct {
	cosmosdbapi.CosmosDocument
	StaleDeviceList staleDeviceListCosmos `json:"mx_keyserver_stale_device_list"`
}

// const upsertStaleDeviceListSQL = "" +
// 	"INSERT INTO keyserver_stale_device_lists (user_id, domain, is_stale, ts_added_secs)" +
// 	" VALUES ($1, $2, $3, $4)" +
// 	" ON CONFLICT (user_id)" +
// 	" DO UPDATE SET is_stale = $3, ts_added_secs = $4"

// "SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1 AND domain = $2"
const selectStaleDeviceListsWithDomainsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_keyserver_stale_device_list.is_stale = @x2 " +
	"and c.mx_keyserver_stale_device_list.domain = @x3 "

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

func (s *staleDeviceListsStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *staleDeviceListsStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getStaleDeviceList(s *staleDeviceListsStatements, ctx context.Context, pk string, docId string) (*staleDeviceListCosmosData, error) {
	response := staleDeviceListCosmosData{}
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

	//     user_id TEXT PRIMARY KEY NOT NULL,
	docId := userID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getStaleDeviceList(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.StaleDeviceList.IsStale = isStale
		dbData.StaleDeviceList.AddedSecs = time.Now().Unix()
	} else {
		data := staleDeviceListCosmos{
			Domain:  string(domain),
			IsStale: isStale,
			UserID:  userID,
		}

		dbData = &staleDeviceListCosmosData{
			CosmosDocument:  cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			StaleDeviceList: data,
		}
	}
	// _, err := s.upsertKeyChangeStmt.ExecContext(ctx, partition, offset, userID)
	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func (s *staleDeviceListsStatements) SelectUserIDsWithStaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error) {
	// we only query for 1 domain or all domains so optimise for those use cases
	if len(domains) == 0 {

		// "SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1"
		// rows, err := s.selectStaleDeviceListsStmt.QueryContext(ctx, true)
		params := map[string]interface{}{
			"@x1": s.getCollectionName(),
			"@x2": true,
		}

		var rows []staleDeviceListCosmosData
		err := cosmosdbapi.PerformQuery(ctx,
			s.db.connection,
			s.db.cosmosConfig.DatabaseName,
			s.db.cosmosConfig.ContainerName,
			s.getPartitionKey(), s.selectStaleDeviceListsStmt, params, &rows)

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
			"@x1": s.getCollectionName(),
			"@x2": true,
			"@x3": string(domain),
		}

		var rows []staleDeviceListCosmosData
		err := cosmosdbapi.PerformQuery(ctx,
			s.db.connection,
			s.db.cosmosConfig.DatabaseName,
			s.db.cosmosConfig.ContainerName,
			s.getPartitionKey(), s.selectStaleDeviceListsWithDomainsStmt, params, &rows)

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

func rowsToUserIDs(ctx context.Context, rows []staleDeviceListCosmosData) (result []string, err error) {
	for _, item := range rows {
		var userID string
		userID = item.StaleDeviceList.UserID
		result = append(result, userID)
	}
	return result, nil
}
