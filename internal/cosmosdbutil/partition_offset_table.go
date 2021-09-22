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

package cosmosdbutil

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/gomatrixserverlib"
)

// // A PartitionOffset is the offset into a partition of the input log.
// type PartitionOffset struct {
// 	// The ID of the partition.
// 	Partition int32
// 	// The offset into the partition.
// 	Offset int64
// }

// const partitionOffsetsSchema = `
// -- The offsets that the server has processed up to.
// CREATE TABLE IF NOT EXISTS ${prefix}_partition_offsets (
//     -- The name of the topic.
//     topic TEXT NOT NULL,
//     -- The 32-bit partition ID
//     partition INTEGER NOT NULL,
//     -- The 64-bit offset.
//     partition_offset BIGINT NOT NULL,
//     UNIQUE (topic, partition)
// );
// `

type partitionOffsetCosmos struct {
	Topic           string `json:"topic"`
	Partition       int32  `json:"partition"`
	PartitionOffset int64  `json:"partition_offset"`
}

type partitionOffsetCosmosData struct {
	cosmosdbapi.CosmosDocument
	PartitionOffset partitionOffsetCosmos `json:"mx_partition_offset"`
}

// "SELECT partition, partition_offset FROM ${prefix}_partition_offsets WHERE topic = $1"
const selectPartitionOffsetsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_partition_offset.topic = @x2 "

// const upsertPartitionOffsetsSQL = "" +
// 	"INSERT INTO ${prefix}_partition_offsets (topic, partition, partition_offset) VALUES ($1, $2, $3)" +
// 	" ON CONFLICT (topic, partition)" +
// 	" DO UPDATE SET partition_offset = $3"

type Database struct {
	Connection   cosmosdbapi.CosmosConnection
	DatabaseName string
	CosmosConfig cosmosdbapi.CosmosConfig
	ServerName   gomatrixserverlib.ServerName
}

// PartitionOffsetStatements represents a set of statements that can be run on a partition_offsets table.
type PartitionOffsetStatements struct {
	db                         *Database
	writer                     Writer
	selectPartitionOffsetsStmt string
	// upsertPartitionOffsetStmt  *sql.Stmt
	prefix    string
	tableName string
}

func (s PartitionOffsetStatements) getCollectionName() string {
	// Include the Prefix
	tableName := fmt.Sprintf("%s_%s", s.prefix, s.tableName)
	return cosmosdbapi.GetCollectionName(s.db.DatabaseName, tableName)
}

func (s *PartitionOffsetStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.CosmosConfig.TenantName, s.getCollectionName())
}

func getPartitionOffset(s *PartitionOffsetStatements, ctx context.Context, pk string, docId string) (*partitionOffsetCosmosData, error) {
	response := partitionOffsetCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.Connection,
		s.db.CosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, nil
	}

	return &response, err
}

// Prepare converts the raw SQL statements into prepared statements.
// Takes a prefix to prepend to the table name used to store the partition offsets.
// This allows multiple components to share the same database schema.
func (s *PartitionOffsetStatements) Prepare(db *Database, writer Writer, prefix string) (err error) {
	s.db = db
	s.writer = writer
	s.selectPartitionOffsetsStmt = selectPartitionOffsetsSQL
	s.prefix = prefix
	s.tableName = "partition_offsets"
	return
}

// PartitionOffsets implements PartitionStorer
func (s *PartitionOffsetStatements) PartitionOffsets(
	ctx context.Context, topic string,
) ([]sqlutil.PartitionOffset, error) {
	return s.selectPartitionOffsets(ctx, topic)
}

// SetPartitionOffset implements PartitionStorer
func (s *PartitionOffsetStatements) SetPartitionOffset(
	ctx context.Context, topic string, partition int32, offset int64,
) error {
	return s.upsertPartitionOffset(ctx, topic, partition, offset)
}

// selectPartitionOffsets returns all the partition offsets for the given topic.
func (s *PartitionOffsetStatements) selectPartitionOffsets(
	ctx context.Context, topic string,
) (results []sqlutil.PartitionOffset, err error) {

	// "SELECT partition, partition_offset FROM ${prefix}_partition_offsets WHERE topic = $1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": topic,
	}

	var rows []partitionOffsetCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.Connection,
		s.db.CosmosConfig.DatabaseName,
		s.db.CosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectPartitionOffsetsStmt, params, &rows)

	// rows, err := s.selectPartitionOffsetsStmt.QueryContext(ctx, topic)
	if err != nil {
		return nil, err
	}
	for _, item := range rows {
		var offset sqlutil.PartitionOffset
		// if err = rows.Scan(&offset.Partition, &offset.Offset); err != nil {
		// 	return nil, err
		// }
		offset.Partition = item.PartitionOffset.Partition
		offset.Offset = item.PartitionOffset.PartitionOffset
		results = append(results, offset)
	}
	return results, nil
}

// checkNamedErr calls fn and overwrite err if it was nil and fn returned non-nil
func checkNamedErr(fn func() error, err *error) {
	if e := fn(); e != nil && *err == nil {
		*err = e
	}
}

// UpsertPartitionOffset updates or inserts the partition offset for the given topic.
func (s *PartitionOffsetStatements) upsertPartitionOffset(
	ctx context.Context, topic string, partition int32, offset int64,
) error {
	return s.writer.Do(nil, nil, func(txn *sql.Tx) error {

		// "INSERT INTO ${prefix}_partition_offsets (topic, partition, partition_offset) VALUES ($1, $2, $3)" +
		// " ON CONFLICT (topic, partition)" +
		// " DO UPDATE SET partition_offset = $3"

		// stmt := TxStmt(txn, s.upsertPartitionOffsetStmt)

		//     UNIQUE (topic, partition)
		docId := fmt.Sprintf("%s_%d", topic, partition)
		cosmosDocId := cosmosdbapi.GetDocumentId(s.db.CosmosConfig.TenantName, s.getCollectionName(), docId)

		dbData, _ := getPartitionOffset(s, ctx, s.getPartitionKey(), cosmosDocId)
		if dbData != nil {
			dbData.SetUpdateTime()
			dbData.PartitionOffset.PartitionOffset = offset
		} else {
			data := partitionOffsetCosmos{
				Partition:       partition,
				PartitionOffset: offset,
				Topic:           topic,
			}

			dbData = &partitionOffsetCosmosData{
				CosmosDocument:  cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.CosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
				PartitionOffset: data,
			}

		}

		// _, err := stmt.ExecContext(ctx, topic, partition, offset)

		return cosmosdbapi.UpsertDocument(ctx,
			s.db.Connection,
			s.db.CosmosConfig.DatabaseName,
			s.db.CosmosConfig.ContainerName,
			dbData.Pk,
			&dbData)
	})
}
