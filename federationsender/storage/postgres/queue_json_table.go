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

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const queueJSONSchema = `
-- The queue_retry_json table contains event contents that
-- we failed to send. 
CREATE TABLE IF NOT EXISTS federationsender_queue_json (
	-- The JSON NID. This allows the federationsender_queue_retry table to
	-- cross-reference to find the JSON blob.
	json_nid BIGSERIAL,
	-- The JSON body. Text so that we preserve UTF-8.
	json_body TEXT NOT NULL
);
`

const insertJSONSQL = "" +
	"INSERT INTO federationsender_queue_json (json_body)" +
	" VALUES ($1)" +
	" RETURNING json_nid"

const deleteJSONSQL = "" +
	"DELETE FROM federationsender_queue_json WHERE json_nid = ANY($1)"

const selectJSONSQL = "" +
	"SELECT json_nid, json_body FROM federationsender_queue_json" +
	" WHERE json_nid = ANY($1)"

type queueJSONStatements struct {
	insertJSONStmt *sql.Stmt
	deleteJSONStmt *sql.Stmt
	selectJSONStmt *sql.Stmt
}

func (s *queueJSONStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(queueJSONSchema)
	if err != nil {
		return
	}
	if s.insertJSONStmt, err = db.Prepare(insertJSONSQL); err != nil {
		return
	}
	if s.deleteJSONStmt, err = db.Prepare(deleteJSONSQL); err != nil {
		return
	}
	if s.selectJSONStmt, err = db.Prepare(selectJSONSQL); err != nil {
		return
	}
	return
}

func (s *queueJSONStatements) insertQueueJSON(
	ctx context.Context, txn *sql.Tx, json string,
) (int64, error) {
	stmt := sqlutil.TxStmt(txn, s.insertJSONStmt)
	var lastid int64
	if err := stmt.QueryRowContext(ctx, json).Scan(&lastid); err != nil {
		return 0, err
	}
	return lastid, nil
}

func (s *queueJSONStatements) deleteQueueJSON(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteJSONStmt)
	_, err := stmt.ExecContext(ctx, pq.StringArray(eventIDs))
	return err
}

func (s *queueJSONStatements) selectJSON(
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
