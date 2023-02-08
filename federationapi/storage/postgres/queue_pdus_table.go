// Copyright 2020 The Matrix.org Foundation C.I.C.
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

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const queuePDUsSchema = `
CREATE TABLE IF NOT EXISTS federationsender_queue_pdus (
    -- The transaction ID that was generated before persisting the event.
	transaction_id TEXT NOT NULL,
    -- The destination server that we will send the event to.
	server_name TEXT NOT NULL,
	-- The JSON NID from the federationsender_queue_pdus_json table.
	json_nid BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_pdus_pdus_json_nid_idx
    ON federationsender_queue_pdus (json_nid, server_name);
CREATE INDEX IF NOT EXISTS federationsender_queue_pdus_json_nid_idx
    ON federationsender_queue_pdus (json_nid);
CREATE INDEX IF NOT EXISTS federationsender_queue_pdus_server_name_idx
    ON federationsender_queue_pdus (server_name);
`

const insertQueuePDUSQL = "" +
	"INSERT INTO federationsender_queue_pdus (transaction_id, server_name, json_nid)" +
	" VALUES ($1, $2, $3)"

const deleteQueuePDUSQL = "" +
	"DELETE FROM federationsender_queue_pdus WHERE server_name = $1 AND json_nid = ANY($2)"

const selectQueuePDUsSQL = "" +
	"SELECT json_nid FROM federationsender_queue_pdus" +
	" WHERE server_name = $1" +
	" LIMIT $2"

const selectQueuePDUReferenceJSONCountSQL = "" +
	"SELECT COUNT(*) FROM federationsender_queue_pdus" +
	" WHERE json_nid = $1"

const selectQueuePDUsCountSQL = "" +
	"SELECT COUNT(*) FROM federationsender_queue_pdus" +
	" WHERE server_name = $1"

const selectQueuePDUServerNamesSQL = "" +
	"SELECT DISTINCT server_name FROM federationsender_queue_pdus"

type queuePDUsStatements struct {
	db                                   *sql.DB
	insertQueuePDUStmt                   *sql.Stmt
	deleteQueuePDUsStmt                  *sql.Stmt
	selectQueuePDUsStmt                  *sql.Stmt
	selectQueuePDUReferenceJSONCountStmt *sql.Stmt
	selectQueuePDUsCountStmt             *sql.Stmt
	selectQueuePDUServerNamesStmt        *sql.Stmt
}

func NewPostgresQueuePDUsTable(db *sql.DB) (s *queuePDUsStatements, err error) {
	s = &queuePDUsStatements{
		db: db,
	}
	_, err = s.db.Exec(queuePDUsSchema)
	if err != nil {
		return
	}
	if s.insertQueuePDUStmt, err = s.db.Prepare(insertQueuePDUSQL); err != nil {
		return
	}
	if s.deleteQueuePDUsStmt, err = s.db.Prepare(deleteQueuePDUSQL); err != nil {
		return
	}
	if s.selectQueuePDUsStmt, err = s.db.Prepare(selectQueuePDUsSQL); err != nil {
		return
	}
	if s.selectQueuePDUReferenceJSONCountStmt, err = s.db.Prepare(selectQueuePDUReferenceJSONCountSQL); err != nil {
		return
	}
	if s.selectQueuePDUsCountStmt, err = s.db.Prepare(selectQueuePDUsCountSQL); err != nil {
		return
	}
	if s.selectQueuePDUServerNamesStmt, err = s.db.Prepare(selectQueuePDUServerNamesSQL); err != nil {
		return
	}
	return
}

func (s *queuePDUsStatements) InsertQueuePDU(
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

func (s *queuePDUsStatements) DeleteQueuePDUs(
	ctx context.Context, txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	jsonNIDs []int64,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteQueuePDUsStmt)
	_, err := stmt.ExecContext(ctx, serverName, pq.Int64Array(jsonNIDs))
	return err
}

func (s *queuePDUsStatements) SelectQueuePDUReferenceJSONCount(
	ctx context.Context, txn *sql.Tx, jsonNID int64,
) (int64, error) {
	var count int64
	stmt := sqlutil.TxStmt(txn, s.selectQueuePDUReferenceJSONCountStmt)
	err := stmt.QueryRowContext(ctx, jsonNID).Scan(&count)
	if err == sql.ErrNoRows {
		// It's acceptable for there to be no rows referencing a given
		// JSON NID but it's not an error condition. Just return as if
		// there's a zero count.
		return 0, nil
	}
	return count, err
}

func (s *queuePDUsStatements) SelectQueuePDUCount(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName,
) (int64, error) {
	var count int64
	stmt := sqlutil.TxStmt(txn, s.selectQueuePDUsCountStmt)
	err := stmt.QueryRowContext(ctx, serverName).Scan(&count)
	if err == sql.ErrNoRows {
		// It's acceptable for there to be no rows referencing a given
		// JSON NID but it's not an error condition. Just return as if
		// there's a zero count.
		return 0, nil
	}
	return count, err
}

func (s *queuePDUsStatements) SelectQueuePDUs(
	ctx context.Context, txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
	limit int,
) ([]int64, error) {
	stmt := sqlutil.TxStmt(txn, s.selectQueuePDUsStmt)
	rows, err := stmt.QueryContext(ctx, serverName, limit)
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

func (s *queuePDUsStatements) SelectQueuePDUServerNames(
	ctx context.Context, txn *sql.Tx,
) ([]gomatrixserverlib.ServerName, error) {
	stmt := sqlutil.TxStmt(txn, s.selectQueuePDUServerNamesStmt)
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "queueFromStmt: rows.close() failed")
	var result []gomatrixserverlib.ServerName
	for rows.Next() {
		var serverName gomatrixserverlib.ServerName
		if err = rows.Scan(&serverName); err != nil {
			return nil, err
		}
		result = append(result, serverName)
	}

	return result, rows.Err()
}
