package storage

import (
	"database/sql"
)

// a statementList is a list of SQL statements to prepare and a pointer to where to store the resulting prepared statement.
type statementList []struct {
	statement **sql.Stmt
	sql       string
}

// prepare the SQL for each statement in the list and assign the result to the prepared statement.
func (s statementList) prepare(db *sql.DB) (err error) {
	for _, statement := range s {
		if *statement.statement, err = db.Prepare(statement.sql); err != nil {
			return
		}
	}
	return
}
