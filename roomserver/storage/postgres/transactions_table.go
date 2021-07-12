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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

const transactionsSchema = `
-- The transactions table holds transaction IDs with sender's info and event ID it belongs to.
-- This table is used by roomserver to prevent reprocessing of events.
CREATE TABLE IF NOT EXISTS roomserver_transactions (
	-- The transaction ID of the event.
	transaction_id TEXT NOT NULL,
	-- The session ID of the originating transaction.
	session_id BIGINT NOT NULL,
	-- User ID of the sender who authored the event
	user_id TEXT NOT NULL,
	-- Event ID corresponding to the transaction
	-- Required to return event ID to client on a duplicate request.
	event_id TEXT NOT NULL,
	-- A transaction ID is unique for a user and device
	-- This automatically creates an index.
	PRIMARY KEY (transaction_id, session_id, user_id)
);
`
const insertTransactionSQL = "" +
	"INSERT INTO roomserver_transactions (transaction_id, session_id, user_id, event_id)" +
	" VALUES ($1, $2, $3, $4)"

const selectTransactionEventIDSQL = "" +
	"SELECT event_id FROM roomserver_transactions" +
	" WHERE transaction_id = $1 AND session_id = $2 AND user_id = $3"

type transactionStatements struct {
	insertTransactionStmt        *sql.Stmt
	selectTransactionEventIDStmt *sql.Stmt
}

func createTransactionsTable(db *sql.DB) error {
	_, err := db.Exec(transactionsSchema)
	return err
}

func prepareTransactionsTable(db *sql.DB) (tables.Transactions, error) {
	s := &transactionStatements{}

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
) (err error) {
	_, err = s.insertTransactionStmt.ExecContext(
		ctx, transactionID, sessionID, userID, eventID,
	)
	return
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
