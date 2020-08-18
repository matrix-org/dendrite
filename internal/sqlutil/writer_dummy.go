package sqlutil

import (
	"database/sql"
)

type DummyTransactionWriter struct {
}

func NewDummyTransactionWriter() TransactionWriter {
	return &DummyTransactionWriter{}
}

func (w *DummyTransactionWriter) Do(db *sql.DB, txn *sql.Tx, f func(txn *sql.Tx) error) error {
	if txn == nil {
		return WithTransaction(db, func(txn *sql.Tx) error {
			return f(txn)
		})
	} else {
		return f(txn)
	}
}
