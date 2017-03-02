package storage

import (
	"database/sql"
)

type statements struct {
	userIPStatements
	accessTokenStatements
	usersStatements
}

func (s *statements) prepare(db *sql.DB) error {
	var err error

	if err = s.prepareUserStatements(db); err != nil {
		return err
	}

	return nil
}

func (s *statements) prepareUserStatements(db *sql.DB) error {
	var err error

	if err = s.accessTokenStatements.prepare(db); err != nil {
		return err
	}
	if err = s.userIPStatements.prepare(db); err != nil {
		return err
	}
	if err = s.usersStatements.prepare(db); err != nil {
		return err
	}

	return nil
}
