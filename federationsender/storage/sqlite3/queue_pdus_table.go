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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const queuePDUsSchema = `
CREATE TABLE IF NOT EXISTS federationsender_queue_pdus (
    -- The transaction ID that was generated before persisting the event.
	transaction_id TEXT NOT NULL,
    -- The domain part of the user ID the m.room.member event is for.
	server_name TEXT NOT NULL,
	-- The JSON NID from the federationsender_queue_pdus_json table.
	json_nid BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_pdus_pdus_event_id_idx
    ON federationsender_queue_pdus (event_id, server_name);
`

const insertQueuePDUSQL = "" +
	"INSERT INTO federationsender_queue_pdus (transaction_id, server_name, json_nid)" +
	" VALUES ($1, $2, $3)"

const deleteQueueTransactionPDUsSQL = "" +
	"DELETE FROM federationsender_queue_pdus WHERE server_name = $1 AND transaction_id = $2"

const selectQueueNextTransactionIDSQL = "" +
	"SELECT transaction_id FROM federationsender_queue_pdus" +
	" WHERE server_name = $1" +
	" ORDER BY transaction_id ASC" +
	" LIMIT 1"

const selectQueuePDUsByTransactionSQL = "" +
	"SELECT json_nid FROM federationsender_queue_pdus" +
	" WHERE server_name = $1 AND transaction_id = $2" +
	" LIMIT 50"

type queuePDUsStatements struct {
	insertQueuePDUStmt               *sql.Stmt
	deleteQueueTransactionPDUsStmt   *sql.Stmt
	selectQueueNextTransactionIDStmt *sql.Stmt
	selectQueuePDUsByTransactionStmt *sql.Stmt
}

func (s *queuePDUsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(queuePDUsSchema)
	if err != nil {
		return
	}
	if s.insertQueuePDUStmt, err = db.Prepare(insertQueuePDUSQL); err != nil {
		return
	}
	if s.deleteQueueTransactionPDUsStmt, err = db.Prepare(deleteQueueTransactionPDUsSQL); err != nil {
		return
	}
	if s.selectQueueNextTransactionIDStmt, err = db.Prepare(selectQueueNextTransactionIDSQL); err != nil {
		return
	}
	if s.selectQueuePDUsByTransactionStmt, err = db.Prepare(selectQueuePDUsByTransactionSQL); err != nil {
		return
	}
	return
}

func (s *queuePDUsStatements) insertQueuePDU(
	ctx context.Context,
	txn *sql.Tx,
	transactionID gomatrixserverlib.TransactionID,
	serverName gomatrixserverlib.ServerName,
	nid int64,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertQueuePDUStmt)
	_, err := stmt.ExecContext(
		ctx,
		transactionID, // the transaction ID that we initially attempted
		serverName,    // destination server name
		nid,           // JSON blob NID
	)
	return err
}

func (s *queuePDUsStatements) deleteQueueTransaction(
	ctx context.Context, txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	transactionID gomatrixserverlib.TransactionID,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteQueueTransactionPDUsStmt)
	_, err := stmt.ExecContext(ctx, serverName, transactionID)
	return err
}

func (s *queuePDUsStatements) selectQueueNextTransactionID(
	ctx context.Context, txn *sql.Tx, serverName, sendType string,
) (string, error) {
	var transactionID string
	stmt := sqlutil.TxStmt(txn, s.selectQueueNextTransactionIDStmt)
	err := stmt.QueryRowContext(ctx, serverName).Scan(&transactionID)
	return transactionID, err
}

func (s *queuePDUsStatements) selectQueuePDUs(
	ctx context.Context, txn *sql.Tx, serverName string, transactionID string, limit int,
) ([]int64, error) {
	stmt := sqlutil.TxStmt(txn, s.selectQueuePDUsByTransactionStmt)
	rows, err := stmt.QueryContext(ctx, serverName, transactionID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "queueFromStmt: rows.close() failed")
	var result []int64
	for rows.Next() {
		var nid int64
		if err = rows.Scan(&nid); err != nil {
			return nil, err
		}
		result = append(result, nid)
	}

	return result, rows.Err()
}
