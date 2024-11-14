// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
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

const relayQueueSchema = `
CREATE TABLE IF NOT EXISTS relayapi_queue (
	-- The transaction ID that was generated before persisting the event.
	transaction_id TEXT NOT NULL,
	-- The destination server that we will send the event to.
	server_name TEXT NOT NULL,
	-- The JSON NID from the relayapi_queue_json table.
	json_nid BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS relayapi_queue_queue_json_nid_idx
	ON relayapi_queue (json_nid, server_name);
CREATE INDEX IF NOT EXISTS relayapi_queue_json_nid_idx
	ON relayapi_queue (json_nid);
CREATE INDEX IF NOT EXISTS relayapi_queue_server_name_idx
	ON relayapi_queue (server_name);
`

const insertQueueEntrySQL = "" +
	"INSERT INTO relayapi_queue (transaction_id, server_name, json_nid)" +
	" VALUES ($1, $2, $3)"

const deleteQueueEntriesSQL = "" +
	"DELETE FROM relayapi_queue WHERE server_name = $1 AND json_nid = ANY($2)"

const selectQueueEntriesSQL = "" +
	"SELECT json_nid FROM relayapi_queue" +
	" WHERE server_name = $1" +
	" ORDER BY json_nid" +
	" LIMIT $2"

const selectQueueEntryCountSQL = "" +
	"SELECT COUNT(*) FROM relayapi_queue" +
	" WHERE server_name = $1"

type relayQueueStatements struct {
	db                        *sql.DB
	insertQueueEntryStmt      *sql.Stmt
	deleteQueueEntriesStmt    *sql.Stmt
	selectQueueEntriesStmt    *sql.Stmt
	selectQueueEntryCountStmt *sql.Stmt
}

func NewPostgresRelayQueueTable(
	db *sql.DB,
) (s *relayQueueStatements, err error) {
	s = &relayQueueStatements{
		db: db,
	}
	_, err = s.db.Exec(relayQueueSchema)
	if err != nil {
		return
	}

	return s, sqlutil.StatementList{
		{&s.insertQueueEntryStmt, insertQueueEntrySQL},
		{&s.deleteQueueEntriesStmt, deleteQueueEntriesSQL},
		{&s.selectQueueEntriesStmt, selectQueueEntriesSQL},
		{&s.selectQueueEntryCountStmt, selectQueueEntryCountSQL},
	}.Prepare(db)
}

func (s *relayQueueStatements) InsertQueueEntry(
	ctx context.Context,
	txn *sql.Tx,
	transactionID gomatrixserverlib.TransactionID,
	serverName spec.ServerName,
	nid int64,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertQueueEntryStmt)
	_, err := stmt.ExecContext(
		ctx,
		transactionID, // the transaction ID that we initially attempted
		serverName,    // destination server name
		nid,           // JSON blob NID
	)
	return err
}

func (s *relayQueueStatements) DeleteQueueEntries(
	ctx context.Context,
	txn *sql.Tx,
	serverName spec.ServerName,
	jsonNIDs []int64,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteQueueEntriesStmt)
	_, err := stmt.ExecContext(ctx, serverName, pq.Int64Array(jsonNIDs))
	return err
}

func (s *relayQueueStatements) SelectQueueEntries(
	ctx context.Context,
	txn *sql.Tx,
	serverName spec.ServerName,
	limit int,
) ([]int64, error) {
	stmt := sqlutil.TxStmt(txn, s.selectQueueEntriesStmt)
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

func (s *relayQueueStatements) SelectQueueEntryCount(
	ctx context.Context,
	txn *sql.Tx,
	serverName spec.ServerName,
) (int64, error) {
	var count int64
	stmt := sqlutil.TxStmt(txn, s.selectQueueEntryCountStmt)
	err := stmt.QueryRowContext(ctx, serverName).Scan(&count)
	if err == sql.ErrNoRows {
		// It's acceptable for there to be no rows referencing a given
		// JSON NID but it's not an error condition. Just return as if
		// there's a zero count.
		return 0, nil
	}
	return count, err
}
