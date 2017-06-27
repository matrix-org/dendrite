package storage

import (
	"database/sql"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationsender/types"
)

// Database stores information needed by the federation sender
type Database struct {
	joinedHostsStatements
	roomStatements
	common.PartitionOffsetStatements
	db *sql.DB
}

// NewDatabase opens a new database
func NewDatabase(dataSourceName string) (*Database, error) {
	var result Database
	var err error
	if result.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	if err = result.prepare(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *Database) prepare() error {
	var err error

	if err = d.joinedHostsStatements.prepare(d.db); err != nil {
		return err
	}

	if err = d.roomStatements.prepare(d.db); err != nil {
		return err
	}

	if err = d.PartitionOffsetStatements.Prepare(d.db); err != nil {
		return err
	}

	return nil
}

// PartitionOffsets implements common.PartitionStorer
func (d *Database) PartitionOffsets(topic string) ([]common.PartitionOffset, error) {
	return d.SelectPartitionOffsets(topic)
}

// SetPartitionOffset implements common.PartitionStorer
func (d *Database) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.UpsertPartitionOffset(topic, partition, offset)
}

// UpdateRoom updates the joined hosts for a room.
func (d *Database) UpdateRoom(
	roomID, oldEventID, newEventID string,
	addHosts []types.JoinedHost,
	removeHosts []string,
) (joinedHosts []types.JoinedHost, err error) {
	err = runTransaction(d.db, func(txn *sql.Tx) error {
		if err = d.insertRoom(txn, roomID); err != nil {
			return err
		}
		lastSentEventID, err := d.selectRoomForUpdate(txn, roomID)
		if err != nil {
			return err
		}
		if lastSentEventID != oldEventID {
			return types.EventIDMismatchError{lastSentEventID, oldEventID}
		}
		joinedHosts, err = d.selectJoinedHosts(txn, roomID)
		if err != nil {
			return err
		}
		for _, add := range addHosts {
			err = d.insertJoinedHosts(txn, roomID, add.EventID, add.ServerName)
			if err != nil {
				return err
			}
		}
		if err = d.deleteJoinedHosts(txn, removeHosts); err != nil {
			return err
		}
		return d.updateRoom(txn, roomID, newEventID)
	})
	return
}

func runTransaction(db *sql.DB, fn func(txn *sql.Tx) error) (err error) {
	txn, err := db.Begin()
	if err != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			txn.Rollback()
			panic(r)
		} else if err != nil {
			txn.Rollback()
		} else {
			err = txn.Commit()
		}
	}()
	err = fn(txn)
	return
}
