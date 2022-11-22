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

package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const transactionJSONSchema = `
-- The federationsender_transaction_json table contains event contents that
-- we are storing for future forwarding. 
CREATE TABLE IF NOT EXISTS federationsender_transaction_json (
	-- The JSON NID. This allows cross-referencing to find the JSON blob.
	json_nid BIGSERIAL,
	-- The JSON body. Text so that we preserve UTF-8.
	json_body TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_transaction_json_json_nid_idx
    ON federationsender_transaction_json (json_nid);
`

const insertTransactionJSONSQL = "" +
	"INSERT INTO federationsender_transaction_json (json_body)" +
	" VALUES ($1)" +
	" RETURNING json_nid"

const deleteTransactionJSONSQL = "" +
	"DELETE FROM federationsender_transaction_json WHERE json_nid = ANY($1)"

const selectTransactionJSONSQL = "" +
	"SELECT json_nid, json_body FROM federationsender_transaction_json" +
	" WHERE json_nid = ANY($1)"

type transactionJSONStatements struct {
	db             *sql.DB
	insertJSONStmt *sql.Stmt
	deleteJSONStmt *sql.Stmt
	selectJSONStmt *sql.Stmt
}

func NewPostgresTransactionJSONTable(db *sql.DB) (s *transactionJSONStatements, err error) {
	s = &transactionJSONStatements{
		db: db,
	}
	_, err = s.db.Exec(transactionJSONSchema)
	if err != nil {
		return
	}
	if s.insertJSONStmt, err = s.db.Prepare(insertTransactionJSONSQL); err != nil {
		return
	}
	if s.deleteJSONStmt, err = s.db.Prepare(deleteTransactionJSONSQL); err != nil {
		return
	}
	if s.selectJSONStmt, err = s.db.Prepare(selectTransactionJSONSQL); err != nil {
		return
	}
	return
}

func (s *transactionJSONStatements) InsertTransactionJSON(
	ctx context.Context, txn *sql.Tx, json string,
) (int64, error) {
	stmt := sqlutil.TxStmt(txn, s.insertJSONStmt)
	var lastid int64
	if err := stmt.QueryRowContext(ctx, json).Scan(&lastid); err != nil {
		return 0, err
	}
	return lastid, nil
}

func (s *transactionJSONStatements) DeleteTransactionJSON(
	ctx context.Context, txn *sql.Tx, nids []int64,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteJSONStmt)
	_, err := stmt.ExecContext(ctx, pq.Int64Array(nids))
	return err
}

func (s *transactionJSONStatements) SelectTransactionJSON(
	ctx context.Context, txn *sql.Tx, jsonNIDs []int64,
) (map[int64][]byte, error) {
	blobs := map[int64][]byte{}
	stmt := sqlutil.TxStmt(txn, s.selectJSONStmt)
	rows, err := stmt.QueryContext(ctx, pq.Int64Array(jsonNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJSON: rows.close() failed")
	for rows.Next() {
		var nid int64
		var blob []byte
		if err = rows.Scan(&nid, &blob); err != nil {
			return nil, err
		}
		blobs[nid] = blob
	}
	return blobs, err
}
