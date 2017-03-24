package storage

import (
	"database/sql"

	"github.com/matrix-org/dendrite/common"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
)

// SyncServerDatabase represents a sync server database
type SyncServerDatabase struct {
	db         *sql.DB
	partitions common.PartitionOffsetStatements
}

// NewSyncServerDatabase creates a new sync server database
func NewSyncServerDatabase(dataSourceName string) (*SyncServerDatabase, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	partitions := common.PartitionOffsetStatements{}
	if err := partitions.Prepare(db); err != nil {
		return nil, err
	}
	return &SyncServerDatabase{db, partitions}, nil
}

// PartitionOffsets implements common.PartitionStorer
func (d *SyncServerDatabase) PartitionOffsets(topic string) ([]common.PartitionOffset, error) {
	return d.partitions.SelectPartitionOffsets(topic)
}

// SetPartitionOffset implements common.PartitionStorer
func (d *SyncServerDatabase) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.partitions.UpsertPartitionOffset(topic, partition, offset)
}
