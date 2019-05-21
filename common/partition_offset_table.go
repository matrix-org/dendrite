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

package common

import (
	"context"
	"database/sql"
	"strings"
)

const partitionOffsetsSchema = `
-- The offsets that the server has processed up to.
CREATE TABLE IF NOT EXISTS ${prefix}_partition_offsets (
    -- The name of the topic.
    topic TEXT NOT NULL,
    -- The 32-bit partition ID
    partition INTEGER NOT NULL,
    -- The 64-bit offset.
    partition_offset BIGINT NOT NULL,
    CONSTRAINT ${prefix}_topic_partition_unique UNIQUE (topic, partition)
);
`

const selectPartitionOffsetsSQL = "" +
	"SELECT partition, partition_offset FROM ${prefix}_partition_offsets WHERE topic = $1"

const upsertPartitionOffsetsSQL = "" +
	"INSERT INTO ${prefix}_partition_offsets (topic, partition, partition_offset) VALUES ($1, $2, $3)" +
	" ON CONFLICT ON CONSTRAINT ${prefix}_topic_partition_unique" +
	" DO UPDATE SET partition_offset = $3"

// PartitionOffsetStatements represents a set of statements that can be run on a partition_offsets table.
type PartitionOffsetStatements struct {
	selectPartitionOffsetsStmt *sql.Stmt
	upsertPartitionOffsetStmt  *sql.Stmt
}

// Prepare converts the raw SQL statements into prepared statements.
// Takes a prefix to prepend to the table name used to store the partition offsets.
// This allows multiple components to share the same database schema.
// nolint: safesql
func (s *PartitionOffsetStatements) Prepare(db *sql.DB, prefix string) (err error) {
	_, err = db.Exec(strings.Replace(partitionOffsetsSchema, "${prefix}", prefix, -1))
	if err != nil {
		return
	}
	if s.selectPartitionOffsetsStmt, err = db.Prepare(
		strings.Replace(selectPartitionOffsetsSQL, "${prefix}", prefix, -1),
	); err != nil {
		return
	}
	if s.upsertPartitionOffsetStmt, err = db.Prepare(
		strings.Replace(upsertPartitionOffsetsSQL, "${prefix}", prefix, -1),
	); err != nil {
		return
	}
	return
}

// PartitionOffsets implements PartitionStorer
func (s *PartitionOffsetStatements) PartitionOffsets(
	ctx context.Context, topic string,
) ([]PartitionOffset, error) {
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
) ([]PartitionOffset, error) {
	rows, err := s.selectPartitionOffsetsStmt.QueryContext(ctx, topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	var results []PartitionOffset
	for rows.Next() {
		var offset PartitionOffset
		if err := rows.Scan(&offset.Partition, &offset.Offset); err != nil {
			return nil, err
		}
		results = append(results, offset)
	}
	return results, nil
}

// UpsertPartitionOffset updates or inserts the partition offset for the given topic.
func (s *PartitionOffsetStatements) upsertPartitionOffset(
	ctx context.Context, topic string, partition int32, offset int64,
) error {
	_, err := s.upsertPartitionOffsetStmt.ExecContext(ctx, topic, partition, offset)
	return err
}
