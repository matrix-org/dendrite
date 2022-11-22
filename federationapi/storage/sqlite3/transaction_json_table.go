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
)

const transactionJSONSchema = `
-- The federationsender_transaction_json table contains event contents that
-- we are storing for future forwarding. 
CREATE TABLE IF NOT EXISTS federationsender_transaction_json (
	-- The JSON NID. This allows cross-referencing to find the JSON blob.
	json_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	-- The JSON body. Text so that we preserve UTF-8.
	json_body TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_transaction_json_json_nid_idx
    ON federationsender_transaction_json (json_nid);
`

const insertTransactionJSONSQL = "" +
	"INSERT INTO federationsender_transaction_json (json_body)" +
	" VALUES ($1)"

const deleteTransactionJSONSQL = "" +
	"DELETE FROM federationsender_transaction_json WHERE json_nid IN ($1)"

const selectTransactionJSONSQL = "" +
	"SELECT json_nid, json_body FROM federationsender_transaction_json" +
	" WHERE json_nid IN ($1)"

type transactionJSONStatements struct {
	db             *sql.DB
	insertJSONStmt *sql.Stmt
	//deleteJSONStmt *sql.Stmt - prepared at runtime due to variadic
	//selectJSONStmt *sql.Stmt - prepared at runtime due to variadic
}

func NewSQLiteTransactionJSONTable(db *sql.DB) (s *transactionJSONStatements, err error) {
	s = &transactionJSONStatements{
		db: db,
	}
	_, err = db.Exec(transactionJSONSchema)
	if err != nil {
		return
	}
	if s.insertJSONStmt, err = db.Prepare(insertTransactionJSONSQL); err != nil {
		return
	}
	return
}

func (s *transactionJSONStatements) InsertTransactionJSON(
	ctx context.Context, txn *sql.Tx, json string,
) (lastid int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.insertJSONStmt)
	res, err := stmt.ExecContext(ctx, json)
	if err != nil {
		return 0, fmt.Errorf("stmt.QueryContext: %w", err)
	}
	lastid, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("res.LastInsertId: %w", err)
	}
	return
}

func (s *transactionJSONStatements) DeleteTransactionJSON(
	ctx context.Context, txn *sql.Tx, nids []int64,
) error {
	deleteSQL := strings.Replace(deleteTransactionJSONSQL, "($1)", sqlutil.QueryVariadic(len(nids)), 1)
	deleteStmt, err := txn.Prepare(deleteSQL)
	if err != nil {
		return fmt.Errorf("s.deleteTransactionJSON s.db.Prepare: %w", err)
	}

	iNIDs := make([]interface{}, len(nids))
	for k, v := range nids {
		iNIDs[k] = v
	}

	stmt := sqlutil.TxStmt(txn, deleteStmt)
	_, err = stmt.ExecContext(ctx, iNIDs...)
	return err
}

func (s *transactionJSONStatements) SelectTransactionJSON(
	ctx context.Context, txn *sql.Tx, jsonNIDs []int64,
) (map[int64][]byte, error) {
	selectSQL := strings.Replace(selectTransactionJSONSQL, "($1)", sqlutil.QueryVariadic(len(jsonNIDs)), 1)
	selectStmt, err := txn.Prepare(selectSQL)
	if err != nil {
		return nil, fmt.Errorf("s.selectTransactionJSON s.db.Prepare: %w", err)
	}

	iNIDs := make([]interface{}, len(jsonNIDs))
	for k, v := range jsonNIDs {
		iNIDs[k] = v
	}

	blobs := map[int64][]byte{}
	stmt := sqlutil.TxStmt(txn, selectStmt)
	rows, err := stmt.QueryContext(ctx, iNIDs...)
	if err != nil {
		return nil, fmt.Errorf("s.selectTransactionJSON stmt.QueryContext: %w", err)
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJSON: rows.close() failed")
	for rows.Next() {
		var nid int64
		var blob []byte
		if err = rows.Scan(&nid, &blob); err != nil {
			return nil, fmt.Errorf("s.selectTransactionJSON rows.Scan: %w", err)
		}
		blobs[nid] = blob
	}
	return blobs, err
}
