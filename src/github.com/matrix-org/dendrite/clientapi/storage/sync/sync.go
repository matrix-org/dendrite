package sync

import (
	"database/sql"

	"github.com/matrix-org/dendrite/common"
)

// Database represents a sync server database
type Database struct {
	db         *sql.DB
	partitions common.PartitionOffsetStatements
}

// NewDatabase creates a new sync server database
func NewDatabase(dataSourceName string) (*Database, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	partitions := common.PartitionOffsetStatements{}
	if err := partitions.Prepare(db); err != nil {
		return nil, err
	}
	return &Database{db, partitions}, nil
}

// PartitionOffsets implements common.PartitionStorer
func (d *Database) PartitionOffsets(topic string) ([]common.PartitionOffset, error) {
	return d.partitions.SelectPartitionOffsets(topic)
}

// SetPartitionOffset implements common.PartitionStorer
func (d *Database) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.partitions.UpsertPartitionOffset(topic, partition, offset)
}
