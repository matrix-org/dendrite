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

const relayQueueJSONSchema = `
-- The relayapi_queue_json table contains event contents that
-- we are storing for future forwarding. 
CREATE TABLE IF NOT EXISTS relayapi_queue_json (
	-- The JSON NID. This allows cross-referencing to find the JSON blob.
	json_nid BIGSERIAL,
	-- The JSON body. Text so that we preserve UTF-8.
	json_body TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS relayapi_queue_json_json_nid_idx
	ON relayapi_queue_json (json_nid);
`

const insertQueueJSONSQL = "" +
	"INSERT INTO relayapi_queue_json (json_body)" +
	" VALUES ($1)" +
	" RETURNING json_nid"

const deleteQueueJSONSQL = "" +
	"DELETE FROM relayapi_queue_json WHERE json_nid = ANY($1)"

const selectQueueJSONSQL = "" +
	"SELECT json_nid, json_body FROM relayapi_queue_json" +
	" WHERE json_nid = ANY($1)"

type relayQueueJSONStatements struct {
	db             *sql.DB
	insertJSONStmt *sql.Stmt
	deleteJSONStmt *sql.Stmt
	selectJSONStmt *sql.Stmt
}

func NewPostgresRelayQueueJSONTable(db *sql.DB) (s *relayQueueJSONStatements, err error) {
	s = &relayQueueJSONStatements{
		db: db,
	}
	_, err = s.db.Exec(relayQueueJSONSchema)
	if err != nil {
		return
	}

	return s, sqlutil.StatementList{
		{&s.insertJSONStmt, insertQueueJSONSQL},
		{&s.deleteJSONStmt, deleteQueueJSONSQL},
		{&s.selectJSONStmt, selectQueueJSONSQL},
	}.Prepare(db)
}

func (s *relayQueueJSONStatements) InsertQueueJSON(
	ctx context.Context, txn *sql.Tx, json string,
) (int64, error) {
	stmt := sqlutil.TxStmt(txn, s.insertJSONStmt)
	var lastid int64
	if err := stmt.QueryRowContext(ctx, json).Scan(&lastid); err != nil {
		return 0, err
	}
	return lastid, nil
}

func (s *relayQueueJSONStatements) DeleteQueueJSON(
	ctx context.Context, txn *sql.Tx, nids []int64,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteJSONStmt)
	_, err := stmt.ExecContext(ctx, pq.Int64Array(nids))
	return err
}

func (s *relayQueueJSONStatements) SelectQueueJSON(
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
	return blobs, rows.Err()
}
