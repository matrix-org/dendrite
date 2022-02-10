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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

const publishedSchema = `
-- Stores which rooms are published in the room directory
CREATE TABLE IF NOT EXISTS roomserver_published (
    -- The room ID of the room
    room_id TEXT NOT NULL PRIMARY KEY,
    -- Whether it is published or not
    published BOOLEAN NOT NULL DEFAULT false
);
`

const upsertPublishedSQL = "" +
	"INSERT INTO roomserver_published (room_id, published) VALUES ($1, $2) " +
	"ON CONFLICT (room_id) DO UPDATE SET published=$2"

const selectAllPublishedSQL = "" +
	"SELECT room_id FROM roomserver_published WHERE published = $1 ORDER BY room_id ASC"

const selectPublishedSQL = "" +
	"SELECT published FROM roomserver_published WHERE room_id = $1"

type publishedStatements struct {
	upsertPublishedStmt    *sql.Stmt
	selectAllPublishedStmt *sql.Stmt
	selectPublishedStmt    *sql.Stmt
}

func createPublishedTable(db *sql.DB) error {
	_, err := db.Exec(publishedSchema)
	return err
}

func preparePublishedTable(db *sql.DB) (tables.Published, error) {
	s := &publishedStatements{}

	return s, sqlutil.StatementList{
		{&s.upsertPublishedStmt, upsertPublishedSQL},
		{&s.selectAllPublishedStmt, selectAllPublishedSQL},
		{&s.selectPublishedStmt, selectPublishedSQL},
	}.Prepare(db)
}

func (s *publishedStatements) UpsertRoomPublished(
	ctx context.Context, txn *sql.Tx, roomID string, published bool,
) (err error) {
	stmt := sqlutil.TxStmt(txn, s.upsertPublishedStmt)
	_, err = stmt.ExecContext(ctx, roomID, published)
	return
}

func (s *publishedStatements) SelectPublishedFromRoomID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (published bool, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectPublishedStmt)
	err = stmt.QueryRowContext(ctx, roomID).Scan(&published)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return
}

func (s *publishedStatements) SelectAllPublishedRooms(
	ctx context.Context, txn *sql.Tx, published bool,
) ([]string, error) {
	stmt := sqlutil.TxStmt(txn, s.selectAllPublishedStmt)
	rows, err := stmt.QueryContext(ctx, published)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectAllPublishedStmt: rows.close() failed")

	var roomIDs []string
	for rows.Next() {
		var roomID string
		if err = rows.Scan(&roomID); err != nil {
			return nil, err
		}

		roomIDs = append(roomIDs, roomID)
	}
	return roomIDs, rows.Err()
}
