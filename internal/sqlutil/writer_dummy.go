package sqlutil

import (
	"database/sql"
)

type DummyWriter struct {
}

func NewDummyWriter() Writer {
	return &DummyWriter{}
}

func (w *DummyWriter) Do(db *sql.DB, txn *sql.Tx, f func(txn *sql.Tx) error) error {
	if db != nil && txn == nil {
		return WithTransaction(db, func(txn *sql.Tx) error {
			return f(txn)
		})
	} else {
		return f(txn)
	}
}
