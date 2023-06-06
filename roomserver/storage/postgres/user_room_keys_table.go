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

package postgres

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/sirupsen/logrus"
)

const userRoomKeysSchema = `
CREATE TABLE IF NOT EXISTS roomserver_user_room_keys (     
    user_nid    INTEGER NOT NULL,
    room_nid    INTEGER NOT NULL,
    pseudo_id_key BYTEA NOT NULL,
    pseudo_id_pub_key BYTEA NOT NULL,
    CONSTRAINT roomserver_user_room_keys_pk PRIMARY KEY (user_nid, room_nid)
);
`

const insertUserRoomKeySQL = `
	INSERT INTO roomserver_user_room_keys (user_nid, room_nid, pseudo_id_key, pseudo_id_pub_key) VALUES ($1, $2, $3, $4)
	ON CONFLICT ON CONSTRAINT roomserver_user_room_keys_pk DO UPDATE SET pseudo_id_key = roomserver_user_room_keys.pseudo_id_key
	RETURNING (pseudo_id_key)
`
const selectUserRoomKeySQL = `SELECT pseudo_id_key FROM roomserver_user_room_keys WHERE user_nid = $1 AND room_nid = $2`

const selectUserNIDsSQL = `SELECT user_nid, pseudo_id_pub_key  FROM roomserver_user_room_keys WHERE pseudo_id_pub_key = ANY($1)`

type userRoomKeysStatements struct {
	insertUserRoomKeyStmt *sql.Stmt
	selectUserRoomKeyStmt *sql.Stmt
	selectUserIDsStmt     *sql.Stmt
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
		{&s.selectUserIDsStmt, selectUserNIDsSQL},
	}.Prepare(db)
}

func (s *userRoomKeysStatements) InsertUserRoomKey(
	ctx context.Context,
	txn *sql.Tx,
	userNID types.EventStateKeyNID,
	roomNID types.RoomNID,
	key ed25519.PrivateKey,
) (result ed25519.PrivateKey, err error) {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.insertUserRoomKeyStmt)
	err = stmt.QueryRowContext(ctx, userNID, roomNID, key, key.Public()).Scan(&result)
	return result, err
}

func (s *userRoomKeysStatements) SelectUserRoomKey(
	ctx context.Context,
	txn *sql.Tx,
	userNID types.EventStateKeyNID,
	roomNID types.RoomNID,
) (ed25519.PrivateKey, error) {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.selectUserRoomKeyStmt)
	var result ed25519.PrivateKey
	err := stmt.QueryRowContext(ctx, userNID, roomNID).Scan(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return result, err
}

func (s *userRoomKeysStatements) BulkSelectUserNIDs(
	ctx context.Context,
	txn *sql.Tx,
	senderKeys [][]byte,
) (map[string]types.EventStateKeyNID, error) {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.selectUserIDsStmt)
	logrus.Infof("%#v", senderKeys)
	rows, err := stmt.QueryContext(ctx, pq.Array(senderKeys))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "failed to close rows")

	result := make(map[string]types.EventStateKeyNID, len(senderKeys))
	var publicKey []byte
	var userNID types.EventStateKeyNID
	for rows.Next() {
		if err = rows.Scan(&userNID, &publicKey); err != nil {
			return nil, err
		}
		result[string(publicKey)] = userNID
	}
	return result, rows.Err()
}
