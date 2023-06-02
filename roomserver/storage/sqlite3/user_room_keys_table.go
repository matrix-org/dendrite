// Copyright 2023 The Matrix.org Foundation C.I.C.
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
	"crypto/ed25519"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const userRoomKeysSchema = `
CREATE TABLE IF NOT EXISTS roomserver_user_room_keys (     
    user_nid    INTEGER NOT NULL,
    room_nid    INTEGER NOT NULL,
    pseudo_id_key TEXT NOT NULL,
    CONSTRAINT roomserver_user_room_keys_pk PRIMARY KEY (user_nid, room_nid)
);
`

const insertUserRoomKeySQL = `INSERT INTO roomserver_user_room_keys (user_nid, room_nid, pseudo_id_key) VALUES ($1, $2, $3)`
const selectUserRoomKeySQL = `SELECT pseudo_id_key FROM roomserver_user_room_keys WHERE user_nid = $1 AND room_nid = $2`

type userRoomKeysStatements struct {
	insertUserRoomKeyStmt *sql.Stmt
	selectUserRoomKeyStmt *sql.Stmt
}

func CreateUserRoomKeysTable(db *sql.DB) error {
	_, err := db.Exec(userRoomKeysSchema)
	return err
}

func PrepareUserRoomKeysTable(db *sql.DB) (tables.UserRoomKeys, error) {
	s := &userRoomKeysStatements{}
	return s, sqlutil.StatementList{
		{&s.insertUserRoomKeyStmt, insertUserRoomKeySQL},
		{&s.selectUserRoomKeyStmt, selectUserRoomKeySQL},
	}.Prepare(db)
}

func (s *userRoomKeysStatements) InsertUserRoomKey(
	ctx context.Context,
	txn *sql.Tx,
	userNID types.EventStateKeyNID,
	roomNID types.RoomNID,
	key ed25519.PrivateKey,
) error {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.insertUserRoomKeyStmt)
	defer internal.CloseAndLogIfError(ctx, stmt, "failed to close statement")
	_, err := stmt.ExecContext(ctx, userNID, roomNID, key)
	return err
}

func (s *userRoomKeysStatements) SelectUserRoomKey(
	ctx context.Context,
	txn *sql.Tx,
	userNID types.EventStateKeyNID,
	roomNID types.RoomNID,
) (ed25519.PrivateKey, error) {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.selectUserRoomKeyStmt)
	defer internal.CloseAndLogIfError(ctx, stmt, "failed to close statement")
	var result ed25519.PrivateKey
	err := stmt.QueryRowContext(ctx, userNID, roomNID).Scan(&result)
	return result, err
}
