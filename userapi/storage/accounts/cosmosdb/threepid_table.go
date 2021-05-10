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

type ThreePIDObject struct {
	Localpart string `json:"local_part"`
	ThreePID  string `json:"three_pid"`
	Medium    string `json:"medium"`
}

type ThreePIDCosmosData struct {
	Id        string         `json:"id"`
	Pk        string         `json:"_pk"`
	Cn        string         `json:"_cn"`
	ETag      string         `json:"_etag"`
	Timestamp int64          `json:"_ts"`
	Object    ThreePIDObject `json:"_object"`
}

type threepidStatements struct {
	db        *Database
	tableName string
}

func (s *threepidStatements) prepare(db *Database) (err error) {
	s.db = db
	s.tableName = "account_threepid"
	return
}

func (s *threepidStatements) selectLocalpartForThreePID(
	ctx context.Context, threepid string, medium string,
) (localpart string, err error) {

	// "SELECT localpart FROM account_threepid WHERE threepid = $1 AND medium = $2"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.threepids.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []ThreePIDCosmosData{}
	var selectLocalPartThreePIDCosmos = "select * from c where c._cn = @x1 and c._object.three_pid = @x2 and c._object.medium = @x3"
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": threepid,
		"@x3": medium,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectLocalPartThreePIDCosmos, params)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		config.DatabaseName,
		config.TenantName,
		query,
		&response,
		options)

	if ex != nil {
		return "", ex
	}

	if len(response) == 0 {
		return "", nil
	}

	return response[0].Object.Localpart, nil
}

func (s *threepidStatements) selectThreePIDsForLocalpart(
	ctx context.Context, localpart string,
) (threepids []authtypes.ThreePID, err error) {

	// "SELECT threepid, medium FROM account_threepid WHERE localpart = $1"
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.threepids.tableName)
	var pk = cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	response := []ThreePIDCosmosData{}
	var selectThreePIDLocalPartCosmos = "select * from c where c._cn = @x1 and c._object.local_part = @x2"
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": localpart,
	}
	var options = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectThreePIDLocalPartCosmos, params)
	var _, ex = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		config.DatabaseName,
		config.TenantName,
		query,
		&response,
		options)

	if ex != nil {
		return threepids, ex
	}

	if len(response) == 0 {
		return threepids, nil
	}

	threepids = []authtypes.ThreePID{}
	for _, item := range response {
		threepids = append(threepids, authtypes.ThreePID{
			Address: item.Object.ThreePID,
			Medium:  item.Object.Medium,
		})
	}
	return threepids, nil
}

func (s *threepidStatements) insertThreePID(
	ctx context.Context, threepid, medium, localpart string,
) (err error) {

	// "INSERT INTO account_threepid (threepid, medium, localpart) VALUES ($1, $2, $3)"
	var result = ThreePIDObject{
		Localpart: localpart,
		Medium:    medium,
		ThreePID:  threepid,
	}

	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)

	id := fmt.Sprintf("%s_%s", threepid, medium)
	var dbData = ThreePIDCosmosData{
		Id:        cosmosdbapi.GetDocumentId(config.TenantName, dbCollectionName, id),
		Cn:        dbCollectionName,
		Pk:        cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName),
		Timestamp: time.Now().Unix(),
		Object:    result,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
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
	var config = cosmosdbapi.DefaultConfig()
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.db.accounts.tableName)
	id := fmt.Sprintf("%s_%s", threepid, medium)
	pk := cosmosdbapi.GetPartitionKey(config.TenantName, dbCollectionName)
	var options = cosmosdbapi.GetDeleteDocumentOptions(pk)
	_, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		config.DatabaseName,
		config.TenantName,
		id,
		options)

	if err != nil {
		return err
	}
	return
}
