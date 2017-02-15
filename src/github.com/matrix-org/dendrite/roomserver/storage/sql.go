package storage

import (
	"database/sql"
)

type statements struct {
	partitionOffsetStatements
	eventTypeStatements
	eventStateKeyStatements
	roomStatements
	eventStatements
	eventJSONStatements
	stateSnapshotStatements
	stateBlockStatements
}

func (s *statements) prepare(db *sql.DB) error {
	var err error

	if err = s.partitionOffsetStatements.prepare(db); err != nil {
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

	return nil
}
