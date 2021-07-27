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
	"fmt"
	"math"
	"time"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

// var keyChangesSchema = `
// -- Stores key change information about users. Used to determine when to send updated device lists to clients.
// CREATE TABLE IF NOT EXISTS keyserver_key_changes (
// 	partition BIGINT NOT NULL,
// 	offset BIGINT NOT NULL,
// 	-- The key owner
// 	user_id TEXT NOT NULL,
// 	UNIQUE (partition, offset)
// );
// `

type KeyChangeCosmos struct {
	Partition int32  `json:"partition"`
	Offset    int64  `json:"_offset"` //offset is reserved
	UserID    string `json:"user_id"`
}

type KeyChangeUserMaxCosmosData struct {
	UserID    string `json:"user_id"`
	MaxOffset int64  `json:"max_offset"`
}

type KeyChangeCosmosData struct {
	Id        string          `json:"id"`
	Pk        string          `json:"_pk"`
	Tn        string          `json:"_sid"`
	Cn        string          `json:"_cn"`
	ETag      string          `json:"_etag"`
	Timestamp int64           `json:"_ts"`
	KeyChange KeyChangeCosmos `json:"mx_keyserver_key_change"`
}

// Replace based on partition|offset - we should never insert duplicates unless the kafka logs are wiped.
// Rather than falling over, just overwrite (though this will mean clients with an existing sync token will
// miss out on updates). TODO: Ideally we would detect when kafka logs are purged then purge this table too.
// const upsertKeyChangeSQL = "" +
// 	"INSERT INTO keyserver_key_changes (partition, offset, user_id)" +
// 	" VALUES ($1, $2, $3)" +
// 	" ON CONFLICT (partition, offset)" +
// 	" DO UPDATE SET user_id = $3"

// select the highest offset for each user in the range. The grouping by user gives distinct entries and then we just
// take the max offset value as the latest offset.
// "SELECT user_id, MAX(offset) FROM keyserver_key_changes WHERE partition = $1 AND offset > $2 AND offset <= $3 GROUP BY user_id"
const selectKeyChangesSQL = "" +
	"select c.mx_keyserver_key_change.user_id as user_id, max(c.mx_keyserver_key_change._offset) as max_offset " +
	"from c where c._sid = @x1 and c._cn = @x2 " +
	"and c.mx_keyserver_key_change.partition = @x3 " +
	"and c.mx_keyserver_key_change._offset > @x4 " +
	"and c.mx_keyserver_key_change._offset < @x5 " +
	"group by c.mx_keyserver_key_change.user_id "

type keyChangesStatements struct {
	db *Database
	// upsertKeyChangeStmt  *sql.Stmt
	selectKeyChangesStmt string
	tableName            string
}

func queryKeyChangeUserMax(s *keyChangesStatements, ctx context.Context, qry string, params map[string]interface{}) ([]KeyChangeUserMaxCosmosData, error) {
	var response []KeyChangeUserMaxCosmosData

	var optionsQry = cosmosdbapi.GetQueryAllPartitionsDocumentsOptions()
	var query = cosmosdbapi.GetQuery(qry, params)
	var _, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	// When there are no Rows we seem to get the generic Bad Req JSON error
	if err != nil {
		// return nil, err
	}

	return response, nil
}

func NewCosmosDBKeyChangesTable(db *Database) (tables.KeyChanges, error) {
	s := &keyChangesStatements{
		db: db,
	}
	s.selectKeyChangesStmt = selectKeyChangesSQL
	s.tableName = "key_changes"
	return s, nil
}

func (s *keyChangesStatements) InsertKeyChange(ctx context.Context, partition int32, offset int64, userID string) error {

	// "INSERT INTO keyserver_key_changes (partition, offset, user_id)" +
	// " VALUES ($1, $2, $3)" +
	// " ON CONFLICT (partition, offset)" +
	// " DO UPDATE SET user_id = $3"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	// 	UNIQUE (partition, offset)
	docId := fmt.Sprintf("%d_%d", partition, offset)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)

	data := KeyChangeCosmos{
		Offset:    offset,
		Partition: partition,
		UserID:    userID,
	}

	dbData := KeyChangeCosmosData{
		Id:        cosmosDocId,
		Tn:        s.db.cosmosConfig.TenantName,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		KeyChange: data,
	}

	// _, err := s.upsertKeyChangeStmt.ExecContext(ctx, partition, offset, userID)
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	var _, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		options)

	return err
}

func (s *keyChangesStatements) SelectKeyChanges(
	ctx context.Context, partition int32, fromOffset, toOffset int64,
) (userIDs []string, latestOffset int64, err error) {
	if toOffset == sarama.OffsetNewest {
		toOffset = math.MaxInt64
	}
	latestOffset = fromOffset

	// "SELECT user_id, MAX(offset) FROM keyserver_key_changes WHERE partition = $1 AND offset > $2 AND offset <= $3 GROUP BY user_id"
	// rows, err := s.selectKeyChangesStmt.QueryContext(ctx, partition, fromOffset, toOffset)

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": s.db.cosmosConfig.TenantName,
		"@x2": dbCollectionName,
		"@x3": partition,
		"@x4": fromOffset,
		"@x5": toOffset,
	}

	response, err := queryKeyChangeUserMax(s, ctx, s.selectKeyChangesStmt, params)

	if err != nil {
		return nil, 0, err
	}

	for _, item := range response {
		var userID string
		var offset int64
		userID = item.UserID
		offset = item.MaxOffset
		if offset > latestOffset {
			latestOffset = offset
		}
		userIDs = append(userIDs, userID)
	}
	return
}
