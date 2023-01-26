// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const relayQueueSchema = `
CREATE TABLE IF NOT EXISTS relayapi_queue (
	-- The transaction ID that was generated before persisting the event.
	transaction_id TEXT NOT NULL,
	-- The domain part of the user ID the m.room.member event is for.
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
	"DELETE FROM relayapi_queue WHERE server_name = $1 AND json_nid IN ($2)"

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
	selectQueueEntriesStmt    *sql.Stmt
	selectQueueEntryCountStmt *sql.Stmt
	// deleteQueueEntriesStmt *sql.Stmt - prepared at runtime due to variadic
}

func NewSQLiteRelayQueueTable(
	db *sql.DB,
) (s *relayQueueStatements, err error) {
	s = &relayQueueStatements{
		db: db,
	}
	_, err = db.Exec(relayQueueSchema)
	if err != nil {
		return
	}

	return s, sqlutil.StatementList{
		{&s.insertQueueEntryStmt, insertQueueEntrySQL},
		{&s.selectQueueEntriesStmt, selectQueueEntriesSQL},
		{&s.selectQueueEntryCountStmt, selectQueueEntryCountSQL},
	}.Prepare(db)
}

func (s *relayQueueStatements) InsertQueueEntry(
	ctx context.Context,
	txn *sql.Tx,
	transactionID gomatrixserverlib.TransactionID,
	serverName gomatrixserverlib.ServerName,
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
	serverName gomatrixserverlib.ServerName,
	jsonNIDs []int64,
) error {
	deleteSQL := strings.Replace(deleteQueueEntriesSQL, "($2)", sqlutil.QueryVariadicOffset(len(jsonNIDs), 1), 1)
	deleteStmt, err := txn.Prepare(deleteSQL)
	if err != nil {
		return fmt.Errorf("s.deleteQueueEntries s.db.Prepare: %w", err)
	}

	params := make([]interface{}, len(jsonNIDs)+1)
	params[0] = serverName
	for k, v := range jsonNIDs {
		params[k+1] = v
	}

	stmt := sqlutil.TxStmt(txn, deleteStmt)
	_, err = stmt.ExecContext(ctx, params...)
	return err
}

func (s *relayQueueStatements) SelectQueueEntries(
	ctx context.Context,
	txn *sql.Tx,
	serverName gomatrixserverlib.ServerName,
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
	serverName gomatrixserverlib.ServerName,
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
