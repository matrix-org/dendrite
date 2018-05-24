// Copyright 2018 New Vector Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"database/sql"
)

const txnIDSchema = `
-- Keeps a count of the current transaction ID per application service 
CREATE TABLE IF NOT EXISTS txn_id_counter (
	-- The ID of the application service the this count belongs to
	as_id TEXT NOT NULL PRIMARY KEY,
	-- The last-used transaction ID
	txn_id INTEGER NOT NULL
);
`

const selectTxnIDSQL = "" +
	"SELECT txn_id FROM txn_id_counter WHERE as_id = $1 LIMIT 1"

const upsertTxnIDSQL = "" +
	"INSERT INTO txn_id_counter(as_id, txn_id) VALUES ($1, $2) " +
	"ON CONFLICT (as_id) DO UPDATE " +
	"SET txn_id = $2"

type txnStatements struct {
	selectTxnIDStmt *sql.Stmt
	upsertTxnIDStmt *sql.Stmt
}

func (s *txnStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(txnIDSchema)
	if err != nil {
		return
	}

	if s.selectTxnIDStmt, err = db.Prepare(selectTxnIDSQL); err != nil {
		return
	}
	if s.upsertTxnIDStmt, err = db.Prepare(upsertTxnIDSQL); err != nil {
		return
	}

	return
}

// selectTxnID inserts a new transactionID mapped to its corresponding
// application service ID into the db.
func (s *txnStatements) selectTxnID(
	ctx context.Context,
	appServiceID string,
) (txnID int, err error) {
	rows, err := s.selectTxnIDStmt.QueryContext(ctx, appServiceID)
	if err != nil {
		return
	}
	defer rows.Close() // nolint: errcheck

	// Scan the TxnID from the database and return
	rows.Next()
	err = rows.Scan(&txnID)
	return
}

// upsertTxnID inserts or updates on existing rows a new transactionID mapped to
// its corresponding application service ID into the db.
func (s *txnStatements) upsertTxnID(
	ctx context.Context,
	appServiceID string,
	txnID int,
) (err error) {
	_, err = s.upsertTxnIDStmt.ExecContext(ctx, appServiceID, txnID)
	return
}
