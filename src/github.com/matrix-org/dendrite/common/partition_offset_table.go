package common

import "database/sql"

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

// PartitionOffsetStatements represents a set of statements that can be run on a partition_offsets table.
type PartitionOffsetStatements struct {
	selectPartitionOffsetsStmt *sql.Stmt
	upsertPartitionOffsetStmt  *sql.Stmt
}

// Prepare converts the raw SQL statements into prepared statements.
func (s *PartitionOffsetStatements) Prepare(db *sql.DB) (err error) {
	_, err = db.Exec(partitionOffsetsSchema)
	if err != nil {
		return
	}
	if s.selectPartitionOffsetsStmt, err = db.Prepare(selectPartitionOffsetsSQL); err != nil {
		return
	}
	if s.upsertPartitionOffsetStmt, err = db.Prepare(upsertPartitionOffsetsSQL); err != nil {
		return
	}
	return
}

// SelectPartitionOffsets returns all the partition offsets for the given topic.
func (s *PartitionOffsetStatements) SelectPartitionOffsets(topic string) ([]PartitionOffset, error) {
	rows, err := s.selectPartitionOffsetsStmt.Query(topic)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []PartitionOffset
	for rows.Next() {
		var offset PartitionOffset
		if err := rows.Scan(&offset.Partition, &offset.Offset); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// UpsertPartitionOffset updates or inserts the partition offset for the given topic.
func (s *PartitionOffsetStatements) UpsertPartitionOffset(topic string, partition int32, offset int64) error {
	_, err := s.upsertPartitionOffsetStmt.Exec(topic, partition, offset)
	return err
}
