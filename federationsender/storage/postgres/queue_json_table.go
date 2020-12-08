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
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

const queueJSONSchema = `
-- The federationsender_queue_json table contains event contents that
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
	db             *sql.DB
	insertJSONStmt *sql.Stmt
	deleteJSONStmt *sql.Stmt
	selectJSONStmt *sql.Stmt
}

func NewPostgresQueueJSONTable(db *sql.DB) (s *queueJSONStatements, err error) {
	s = &queueJSONStatements{
		db: db,
	}
	_, err = s.db.Exec(queueJSONSchema)
	if err != nil {
		return
	}
	if s.insertJSONStmt, err = s.db.Prepare(insertJSONSQL); err != nil {
		return
	}
	if s.deleteJSONStmt, err = s.db.Prepare(deleteJSONSQL); err != nil {
		return
	}
	if s.selectJSONStmt, err = s.db.Prepare(selectJSONSQL); err != nil {
		return
	}
	return
}

func (s *queueJSONStatements) InsertQueueJSON(
	ctx context.Context, txn *sql.Tx, json string,
) (types.ContentNID, error) {
	stmt := sqlutil.TxStmt(txn, s.insertJSONStmt)
	var lastid types.ContentNID
	if err := stmt.QueryRowContext(ctx, json).Scan(&lastid); err != nil {
		return 0, err
	}
	return lastid, nil
}

func (s *queueJSONStatements) DeleteQueueJSON(
	ctx context.Context, txn *sql.Tx, jsonNIDs []types.ContentNID,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteJSONStmt)
	nids := make([]int64, 0, len(jsonNIDs))
	for _, n := range jsonNIDs {
		nids = append(nids, int64(n))
	}
	_, err := stmt.ExecContext(ctx, pq.Int64Array(nids))
	return err
}

func (s *queueJSONStatements) SelectQueueJSON(
	ctx context.Context, txn *sql.Tx, jsonNIDs []types.ContentNID,
) (map[types.ContentNID][]byte, error) {
	blobs := map[types.ContentNID][]byte{}
	stmt := sqlutil.TxStmt(txn, s.selectJSONStmt)
	nids := make([]int64, 0, len(jsonNIDs))
	for _, n := range jsonNIDs {
		nids = append(nids, int64(n))
	}
	rows, err := stmt.QueryContext(ctx, pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectJSON: rows.close() failed")
	for rows.Next() {
		var nid types.ContentNID
		var blob []byte
		if err = rows.Scan(&nid, &blob); err != nil {
			return nil, err
		}
		blobs[nid] = blob
	}
	return blobs, err
}
