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
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

// const threepidSchema = `
// -- Stores data about third party identifiers
// CREATE TABLE IF NOT EXISTS account_threepid (
// 	-- The third party identifier
// 	threepid TEXT NOT NULL,
// 	-- The 3PID medium
// 	medium TEXT NOT NULL DEFAULT 'email',
// 	-- The localpart of the Matrix user ID associated to this 3PID
// 	localpart TEXT NOT NULL,

// 	PRIMARY KEY(threepid, medium)
// );

type ThreePIDCosmos struct {
	Localpart string `json:"local_part"`
	ThreePID  string `json:"three_pid"`
	Medium    string `json:"medium"`
}

type ThreePIDCosmosData struct {
	Id        string         `json:"id"`
	Pk        string         `json:"_pk"`
	Tn        string         `json:"_sid"`
	Cn        string         `json:"_cn"`
	ETag      string         `json:"_etag"`
	Timestamp int64          `json:"_ts"`
	ThreePID  ThreePIDCosmos `json:"mx_userapi_threepid"`
}

type threepidStatements struct {
	db                              *Database
	selectLocalpartForThreePIDStmt  string
	selectThreePIDsForLocalpartStmt string
	// insertThreePIDStmt              *sql.Stmt
	// deleteThreePIDStmt              *sql.Stmt
	tableName string
}

func (s *threepidStatements) prepare(db *Database) (err error) {
	s.db = db
	s.selectLocalpartForThreePIDStmt = "select * from c where c._cn = @x1 and c.mx_userapi_threepid.three_pid = @x2 and c.mx_userapi_threepid.medium = @x3"
	s.selectThreePIDsForLocalpartStmt = "select * from c where c._cn = @x1 and c.mx_userapi_threepid.local_part = @x2"
	s.tableName = "account_threepid"
	return
}

func queryThreePID(s *threepidStatements, ctx context.Context, qry string, params map[string]interface{}) ([]ThreePIDCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []ThreePIDCosmosData

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

func (s *threepidStatements) selectLocalpartForThreePID(
	ctx context.Context, threepid string, medium string,
) (localpart string, err error) {

	// "SELECT localpart FROM account_threepid WHERE threepid = $1 AND medium = $2"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.threepids.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": threepid,
		"@x3": medium,
	}

	response, err := queryThreePID(s, ctx, s.selectLocalpartForThreePIDStmt, params)

	if err != nil {
		return "", err
	}

	if len(response) == 0 {
		return "", nil
	}

	return response[0].ThreePID.Localpart, nil
}

func (s *threepidStatements) selectThreePIDsForLocalpart(
	ctx context.Context, localpart string,
) (threepids []authtypes.ThreePID, err error) {

	// "SELECT threepid, medium FROM account_threepid WHERE localpart = $1"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.threepids.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	response, err := queryThreePID(s, ctx, s.selectThreePIDsForLocalpartStmt, params)

	if err != nil {
		return threepids, err
	}

	if len(response) == 0 {
		return threepids, nil
	}

	threepids = []authtypes.ThreePID{}
	for _, item := range response {
		threepids = append(threepids, authtypes.ThreePID{
			Address: item.ThreePID.ThreePID,
			Medium:  item.ThreePID.Medium,
		})
	}
	return threepids, nil
}

func (s *threepidStatements) insertThreePID(
	ctx context.Context, threepid, medium, localpart string,
) (err error) {

	// "INSERT INTO account_threepid (threepid, medium, localpart) VALUES ($1, $2, $3)"
	var result = ThreePIDCosmos{
		Localpart: localpart,
		Medium:    medium,
		ThreePID:  threepid,
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)

	docId := fmt.Sprintf("%s_%s", threepid, medium)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var dbData = ThreePIDCosmosData{
		Id:        cosmosDocId,
		Tn:        s.db.cosmosConfig.TenantName,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		ThreePID:  result,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	if err != nil {
		return err
	}
	return
}

func (s *threepidStatements) deleteThreePID(
	ctx context.Context, threepid string, medium string) (err error) {

	// "DELETE FROM account_threepid WHERE threepid = $1 AND medium = $2"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)
	docId := fmt.Sprintf("%s_%s", threepid, medium)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var options = cosmosdbapi.GetDeleteDocumentOptions(pk)
	_, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		cosmosDocId,
		options)

	if err != nil {
		return err
	}
	return
}
