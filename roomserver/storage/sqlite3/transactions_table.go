// Copyright 2017-2018 New Vector Ltd
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
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

const transactionsSchema = `
	CREATE TABLE IF NOT EXISTS roomserver_transactions (
		transaction_id TEXT NOT NULL,
		session_id INTEGER NOT NULL,
		user_id TEXT NOT NULL,
		event_id TEXT NOT NULL,
		PRIMARY KEY (transaction_id, session_id, user_id)
	);
`
const insertTransactionSQL = `
	INSERT INTO roomserver_transactions (transaction_id, session_id, user_id, event_id)
	  VALUES ($1, $2, $3, $4)
`

const selectTransactionEventIDSQL = `
	SELECT event_id FROM roomserver_transactions
	  WHERE transaction_id = $1 AND session_id = $2 AND user_id = $3
`

type transactionStatements struct {
	db                           *sql.DB
	insertTransactionStmt        *sql.Stmt
	selectTransactionEventIDStmt *sql.Stmt
}

func NewSqliteTransactionsTable(db *sql.DB) (tables.Transactions, error) {
	s := &transactionStatements{
		db: db,
	}
	_, err := db.Exec(transactionsSchema)
	if err != nil {
		return nil, err
	}

	return s, shared.StatementList{
		{&s.insertTransactionStmt, insertTransactionSQL},
		{&s.selectTransactionEventIDStmt, selectTransactionEventIDSQL},
	}.Prepare(db)
}

func (s *transactionStatements) InsertTransaction(
	ctx context.Context, txn *sql.Tx,
	transactionID string,
	sessionID int64,
	userID string,
	eventID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertTransactionStmt)
	_, err := stmt.ExecContext(
		ctx, transactionID, sessionID, userID, eventID,
	)
	return err
}

func (s *transactionStatements) SelectTransactionEventID(
	ctx context.Context,
	transactionID string,
	sessionID int64,
	userID string,
) (eventID string, err error) {
	err = s.selectTransactionEventIDStmt.QueryRowContext(
		ctx, transactionID, sessionID, userID,
	).Scan(&eventID)
	return
}
