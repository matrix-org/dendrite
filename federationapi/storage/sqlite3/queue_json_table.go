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
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const queueJSONSchema = `
-- The queue_retry_json table contains event contents that
-- we failed to send. 
CREATE TABLE IF NOT EXISTS federationsender_queue_json (
	-- The JSON NID. This allows the federationsender_queue_retry table to
	-- cross-reference to find the JSON blob.
	json_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	-- The JSON body. Text so that we preserve UTF-8.
	json_body TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_json_json_nid_idx
    ON federationsender_queue_json (json_nid);
`

const insertJSONSQL = "" +
	"INSERT INTO federationsender_queue_json (json_body)" +
	" VALUES ($1)"

const deleteJSONSQL = "" +
	"DELETE FROM federationsender_queue_json WHERE json_nid IN ($1)"

const selectJSONSQL = "" +
	"SELECT json_nid, json_body FROM federationsender_queue_json" +
	" WHERE json_nid IN ($1)"

type queueJSONStatements struct {
	db             *sql.DB
	insertJSONStmt *sql.Stmt
	//deleteJSONStmt *sql.Stmt - prepared at runtime due to variadic
	//selectJSONStmt *sql.Stmt - prepared at runtime due to variadic
}

func NewSQLiteQueueJSONTable(db *sql.DB) (s *queueJSONStatements, err error) {
	s = &queueJSONStatements{
		db: db,
	}
	_, err = db.Exec(queueJSONSchema)
	if err != nil {
		return
	}
	if s.insertJSONStmt, err = db.Prepare(insertJSONSQL); err != nil {
		return
	}
	return
}

func (s *queueJSONStatements) InsertQueueJSON(
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

func (s *queueJSONStatements) DeleteQueueJSON(
	ctx context.Context, txn *sql.Tx, nids []int64,
) error {
	deleteSQL := strings.Replace(deleteJSONSQL, "($1)", sqlutil.QueryVariadic(len(nids)), 1)
	deleteStmt, err := txn.Prepare(deleteSQL)
	if err != nil {
		return fmt.Errorf("s.deleteQueueJSON s.db.Prepare: %w", err)
	}

	iNIDs := make([]interface{}, len(nids))
	for k, v := range nids {
		iNIDs[k] = v
	}

	stmt := sqlutil.TxStmt(txn, deleteStmt)
	_, err = stmt.ExecContext(ctx, iNIDs...)
	return err
}

func (s *queueJSONStatements) SelectQueueJSON(
	ctx context.Context, txn *sql.Tx, jsonNIDs []int64,
) (map[int64][]byte, error) {
	selectSQL := strings.Replace(selectJSONSQL, "($1)", sqlutil.QueryVariadic(len(jsonNIDs)), 1)
	selectStmt, err := txn.Prepare(selectSQL)
	if err != nil {
		return nil, fmt.Errorf("s.selectQueueJSON s.db.Prepare: %w", err)
	}

	iNIDs := make([]interface{}, len(jsonNIDs))
	for k, v := range jsonNIDs {
		iNIDs[k] = v
	}

	blobs := map[int64][]byte{}
	stmt := sqlutil.TxStmt(txn, selectStmt)
	rows, err := stmt.QueryContext(ctx, iNIDs...)
	if err != nil {
		return nil, fmt.Errorf("s.selectQueueJSON stmt.QueryContext: %w", err)
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJSON: rows.close() failed")
	for rows.Next() {
		var nid int64
		var blob []byte
		if err = rows.Scan(&nid, &blob); err != nil {
			return nil, fmt.Errorf("s.selectQueueJSON rows.Scan: %w", err)
		}
		blobs[nid] = blob
	}
	return blobs, err
}
