// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/lib/pq"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
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

const selectQueuePDUServerNamesSQL = "" +
	"SELECT DISTINCT server_name FROM federationsender_queue_pdus"

type queuePDUsStatements struct {
	db                                   *sql.DB
	insertQueuePDUStmt                   *sql.Stmt
	deleteQueuePDUsStmt                  *sql.Stmt
	selectQueuePDUsStmt                  *sql.Stmt
	selectQueuePDUReferenceJSONCountStmt *sql.Stmt
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
	return s, sqlutil.StatementList{
		{&s.insertQueuePDUStmt, insertQueuePDUSQL},
		{&s.deleteQueuePDUsStmt, deleteQueuePDUSQL},
		{&s.selectQueuePDUsStmt, selectQueuePDUsSQL},
		{&s.selectQueuePDUReferenceJSONCountStmt, selectQueuePDUReferenceJSONCountSQL},
		{&s.selectQueuePDUServerNamesStmt, selectQueuePDUServerNamesSQL},
	}.Prepare(db)
}

func (s *queuePDUsStatements) InsertQueuePDU(
	ctx context.Context,
	txn *sql.Tx,
	transactionID gomatrixserverlib.TransactionID,
	serverName spec.ServerName,
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
	serverName spec.ServerName,
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

func (s *queuePDUsStatements) SelectQueuePDUs(
	ctx context.Context, txn *sql.Tx,
	serverName spec.ServerName,
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
) ([]spec.ServerName, error) {
	stmt := sqlutil.TxStmt(txn, s.selectQueuePDUServerNamesStmt)
	rows, err := stmt.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "queueFromStmt: rows.close() failed")
	var result []spec.ServerName
	for rows.Next() {
		var serverName spec.ServerName
		if err = rows.Scan(&serverName); err != nil {
			return nil, err
		}
		result = append(result, serverName)
	}

	return result, rows.Err()
}
