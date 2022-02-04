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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

const roomAliasesSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_room_aliases (
    alias TEXT NOT NULL PRIMARY KEY,
    room_id TEXT NOT NULL,
    creator_id TEXT NOT NULL
  );

  CREATE INDEX IF NOT EXISTS roomserver_room_id_idx ON roomserver_room_aliases(room_id);
`

const insertRoomAliasSQL = `
	INSERT INTO roomserver_room_aliases (alias, room_id, creator_id) VALUES ($1, $2, $3)
`

const selectRoomIDFromAliasSQL = `
	SELECT room_id FROM roomserver_room_aliases WHERE alias = $1
`

const selectAliasesFromRoomIDSQL = `
	SELECT alias FROM roomserver_room_aliases WHERE room_id = $1
`

const selectCreatorIDFromAliasSQL = `
	SELECT creator_id FROM roomserver_room_aliases WHERE alias = $1
`

const deleteRoomAliasSQL = `
	DELETE FROM roomserver_room_aliases WHERE alias = $1
`

type roomAliasesStatements struct {
	db                           *sql.DB
	insertRoomAliasStmt          *sql.Stmt
	selectRoomIDFromAliasStmt    *sql.Stmt
	selectAliasesFromRoomIDStmt  *sql.Stmt
	selectCreatorIDFromAliasStmt *sql.Stmt
	deleteRoomAliasStmt          *sql.Stmt
}

func createRoomAliasesTable(db *sql.DB) error {
	_, err := db.Exec(roomAliasesSchema)
	return err
}

func prepareRoomAliasesTable(db *sql.DB) (tables.RoomAliases, error) {
	s := &roomAliasesStatements{
		db: db,
	}

	return s, sqlutil.StatementList{
		{&s.insertRoomAliasStmt, insertRoomAliasSQL},
		{&s.selectRoomIDFromAliasStmt, selectRoomIDFromAliasSQL},
		{&s.selectAliasesFromRoomIDStmt, selectAliasesFromRoomIDSQL},
		{&s.selectCreatorIDFromAliasStmt, selectCreatorIDFromAliasSQL},
		{&s.deleteRoomAliasStmt, deleteRoomAliasSQL},
	}.Prepare(db)
}

func (s *roomAliasesStatements) InsertRoomAlias(
	ctx context.Context, txn *sql.Tx, alias string, roomID string, creatorUserID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertRoomAliasStmt)
	_, err := stmt.ExecContext(ctx, alias, roomID, creatorUserID)
	return err
}

func (s *roomAliasesStatements) SelectRoomIDFromAlias(
	ctx context.Context, txn *sql.Tx, alias string,
) (roomID string, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectRoomIDFromAliasStmt)
	err = stmt.QueryRowContext(ctx, alias).Scan(&roomID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomAliasesStatements) SelectAliasesFromRoomID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (aliases []string, err error) {
	aliases = []string{}
	stmt := sqlutil.TxStmt(txn, s.selectAliasesFromRoomIDStmt)
	rows, err := stmt.QueryContext(ctx, roomID)
	if err != nil {
		return
	}

	defer internal.CloseAndLogIfError(ctx, rows, "selectAliasesFromRoomID: rows.close() failed")

	for rows.Next() {
		var alias string
		if err = rows.Scan(&alias); err != nil {
			return
		}

		aliases = append(aliases, alias)
	}

	return
}

func (s *roomAliasesStatements) SelectCreatorIDFromAlias(
	ctx context.Context, txn *sql.Tx, alias string,
) (creatorID string, err error) {
	stmt := sqlutil.TxStmt(txn, s.selectCreatorIDFromAliasStmt)
	err = stmt.QueryRowContext(ctx, alias).Scan(&creatorID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return
}

func (s *roomAliasesStatements) DeleteRoomAlias(
	ctx context.Context, txn *sql.Tx, alias string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteRoomAliasStmt)
	_, err := stmt.ExecContext(ctx, alias)
	return err
}
