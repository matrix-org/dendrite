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

CREATE UNIQUE INDEX IF NOT EXISTS federationsender_queue_json_json_nid_idx
    ON federationsender_queue_json (json_nid);
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
	return s, sqlutil.StatementList{
		{&s.insertJSONStmt, insertJSONSQL},
		{&s.deleteJSONStmt, deleteJSONSQL},
		{&s.selectJSONStmt, selectJSONSQL},
	}.Prepare(db)
}

func (s *queueJSONStatements) InsertQueueJSON(
	ctx context.Context, txn *sql.Tx, json string,
) (int64, error) {
	stmt := sqlutil.TxStmt(txn, s.insertJSONStmt)
	var lastid int64
	if err := stmt.QueryRowContext(ctx, json).Scan(&lastid); err != nil {
		return 0, err
	}
	return lastid, nil
}

func (s *queueJSONStatements) DeleteQueueJSON(
	ctx context.Context, txn *sql.Tx, nids []int64,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteJSONStmt)
	_, err := stmt.ExecContext(ctx, pq.Int64Array(nids))
	return err
}

func (s *queueJSONStatements) SelectQueueJSON(
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
