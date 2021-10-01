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

type keyChangeCosmos struct {
	Partition int32  `json:"partition"`
	Offset    int64  `json:"_offset"` //offset is reserved
	UserID    string `json:"user_id"`
}

type keyChangeUserMaxCosmosData struct {
	UserID    string `json:"user_id"`
	MaxOffset int64  `json:"max_offset"`
}

type keyChangeCosmosData struct {
	cosmosdbapi.CosmosDocument
	KeyChange keyChangeCosmos `json:"mx_keyserver_key_change"`
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

func (s *keyChangesStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *keyChangesStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getKeyChangeUser(s *keyChangesStatements, ctx context.Context, pk string, docId string) (*keyChangeCosmosData, error) {
	response := keyChangeCosmosData{}
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

	// 	UNIQUE (partition, offset)
	docId := fmt.Sprintf("%d,%d", partition, offset)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getKeyChangeUser(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.KeyChange.UserID = userID
	} else {
		data := keyChangeCosmos{
			Offset:    offset,
			Partition: partition,
			UserID:    userID,
		}

		dbData = &keyChangeCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			KeyChange:      data,
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

func (s *keyChangesStatements) SelectKeyChanges(
	ctx context.Context, partition int32, fromOffset, toOffset int64,
) (userIDs []string, latestOffset int64, err error) {
	if toOffset == sarama.OffsetNewest {
		toOffset = math.MaxInt64
	}
	latestOffset = fromOffset

	// "SELECT user_id, MAX(offset) FROM keyserver_key_changes WHERE partition = $1 AND offset > $2 AND offset <= $3 GROUP BY user_id"
	// rows, err := s.selectKeyChangesStmt.QueryContext(ctx, partition, fromOffset, toOffset)

	params := map[string]interface{}{
		"@x1": s.db.cosmosConfig.TenantName,
		"@x2": s.getCollectionName(),
		"@x3": partition,
		"@x4": fromOffset,
		"@x5": toOffset,
	}

	var rows []keyChangeUserMaxCosmosData
	err = cosmosdbapi.PerformQueryAllPartitions(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.selectKeyChangesStmt, params, &rows)

	if err != nil {
		return nil, 0, err
	}

	for _, item := range rows {
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
