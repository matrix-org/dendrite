package storage

import (
	"database/sql"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	stmts stmts
	db    *sql.DB
}

// Open a postgres database.
func Open(dataSourceName string) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	if err = d.stmts.prepare(d.db); err != nil {
		return nil, err
	}
	return &d, nil
}

// PartitionOffsets implements input.ConsumerDatabase
func (d *Database) PartitionOffsets(topic string) ([]types.PartitionOffset, error) {
	return d.stmts.selectPartitionOffsets(topic)
}

// SetPartitionOffset implements input.ConsumerDatabase
func (d *Database) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.stmts.upsertPartitionOffset(topic, partition, offset)
}
