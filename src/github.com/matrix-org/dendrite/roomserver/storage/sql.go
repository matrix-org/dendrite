package storage

import (
	"database/sql"

	"github.com/matrix-org/dendrite/common"
)

type statements struct {
	common.PartitionOffsetStatements
	eventTypeStatements
	eventStateKeyStatements
	roomStatements
	eventStatements
	eventJSONStatements
	stateSnapshotStatements
	stateBlockStatements
	previousEventStatements
}

func (s *statements) prepare(db *sql.DB) error {
	var err error

	if err = s.PartitionOffsetStatements.Prepare(db); err != nil {
		return err
	}

	if err = s.eventTypeStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventStateKeyStatements.prepare(db); err != nil {
		return err
	}

	if err = s.roomStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventStatements.prepare(db); err != nil {
		return err
	}

	if err = s.eventJSONStatements.prepare(db); err != nil {
		return err
	}

	if err = s.stateSnapshotStatements.prepare(db); err != nil {
		return err
	}

	if err = s.stateBlockStatements.prepare(db); err != nil {
		return err
	}

	if err = s.previousEventStatements.prepare(db); err != nil {
		return err
	}

	return nil
}
