// Copyright 2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const txnIDSchema = `
-- Keeps a count of the current transaction ID
CREATE TABLE IF NOT EXISTS appservice_counters (
  name TEXT PRIMARY KEY NOT NULL,
  last_id INTEGER DEFAULT 1
);
INSERT OR IGNORE INTO appservice_counters (name, last_id) VALUES('txn_id', 1);
`

const selectTxnIDSQL = `
  SELECT last_id FROM appservice_counters WHERE name='txn_id';
  UPDATE appservice_counters SET last_id=last_id+1 WHERE name='txn_id';
`

type txnStatements struct {
	db              *sql.DB
	writer          *sqlutil.TransactionWriter
	selectTxnIDStmt *sql.Stmt
}

func (s *txnStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	s.writer = sqlutil.NewTransactionWriter()
	_, err = db.Exec(txnIDSchema)
	if err != nil {
		return
	}

	if s.selectTxnIDStmt, err = db.Prepare(selectTxnIDSQL); err != nil {
		return
	}

	return
}

// selectTxnID selects the latest ascending transaction ID
func (s *txnStatements) selectTxnID(
	ctx context.Context,
) (txnID int, err error) {
	err = s.writer.Do(s.db, nil, func(txn *sql.Tx) error {
		err := s.selectTxnIDStmt.QueryRowContext(ctx).Scan(&txnID)
		return err
	})
	return
}
