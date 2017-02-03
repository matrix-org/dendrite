package storage

import (
	"database/sql"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type statements struct {
	selectPartitionOffsetsStmt *sql.Stmt
	upsertPartitionOffsetStmt  *sql.Stmt
}

func (s *statements) prepare(db *sql.DB) error {
	var err error

	_, err = db.Exec(partitionOffsetsSchema)
	if err != nil {
		return err
	}

	if s.selectPartitionOffsetsStmt, err = db.Prepare(selectPartitionOffsetsSQL); err != nil {
		return err
	}
	if s.upsertPartitionOffsetStmt, err = db.Prepare(upsertPartitionOffsetsSQL); err != nil {
		return err
	}
	return nil
}

const partitionOffsetsSchema = `
-- The offsets that the server has processed up to.
CREATE TABLE IF NOT EXISTS partition_offsets (
	-- The name of the topic.
	topic TEXT NOT NULL,
	-- The 32-bit partition ID
	partition INTEGER NOT NULL,
	-- The 64-bit offset.
	partition_offset BIGINT NOT NULL,
	CONSTRAINT topic_partition_unique UNIQUE (topic, partition)
);
`

const selectPartitionOffsetsSQL = "" +
	"SELECT partition, partition_offset FROM partition_offsets WHERE topic = $1"

const upsertPartitionOffsetsSQL = "" +
	"INSERT INTO partition_offsets (topic, partition, partition_offset) VALUES ($1, $2, $3)" +
	" ON CONFLICT ON CONSTRAINT topic_partition_unique" +
	" DO UPDATE SET partition_offset = $3"

func (s *statements) selectPartitionOffsets(topic string) ([]types.PartitionOffset, error) {
	rows, err := s.selectPartitionOffsetsStmt.Query(topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []types.PartitionOffset
	for rows.Next() {
		var offset types.PartitionOffset
		if err := rows.Scan(&offset.Partition, &offset.Offset); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (s *statements) upsertPartitionOffset(topic string, partition int32, offset int64) error {
	_, err := s.upsertPartitionOffsetStmt.Exec(topic, partition, offset)
	return err
}
