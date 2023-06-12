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
)

const userRoomKeysSchema = `
CREATE TABLE IF NOT EXISTS roomserver_user_room_keys (     
    user_nid    INTEGER NOT NULL,
    room_nid    INTEGER NOT NULL,
    pseudo_id_key BYTEA NULL, -- may be null for users not local to the server
    pseudo_id_pub_key BYTEA NOT NULL,
    CONSTRAINT roomserver_user_room_keys_pk PRIMARY KEY (user_nid, room_nid)
);
`

const insertUserRoomPrivateKeySQL = `
	INSERT INTO roomserver_user_room_keys (user_nid, room_nid, pseudo_id_key, pseudo_id_pub_key) VALUES ($1, $2, $3, $4)
	ON CONFLICT ON CONSTRAINT roomserver_user_room_keys_pk DO UPDATE SET pseudo_id_key = roomserver_user_room_keys.pseudo_id_key
	RETURNING (pseudo_id_key)
`

const insertUserRoomPublicKeySQL = `
	INSERT INTO roomserver_user_room_keys (user_nid, room_nid, pseudo_id_pub_key) VALUES ($1, $2, $3)
	ON CONFLICT ON CONSTRAINT roomserver_user_room_keys_pk DO UPDATE SET pseudo_id_pub_key = $3
	RETURNING (pseudo_id_pub_key)
`

const selectUserRoomKeySQL = `SELECT pseudo_id_key FROM roomserver_user_room_keys WHERE user_nid = $1 AND room_nid = $2`

const selectUserNIDsSQL = `SELECT user_nid, room_nid, pseudo_id_pub_key FROM roomserver_user_room_keys WHERE room_nid = ANY($1) AND pseudo_id_pub_key = ANY($2)`

type userRoomKeysStatements struct {
	insertUserRoomPrivateKeyStmt *sql.Stmt
	insertUserRoomPublicKeyStmt  *sql.Stmt
	selectUserRoomKeyStmt        *sql.Stmt
	selectUserNIDsStmt           *sql.Stmt
}

func CreateUserRoomKeysTable(db *sql.DB) error {
	_, err := db.Exec(userRoomKeysSchema)
	return err
}

func PrepareUserRoomKeysTable(db *sql.DB) (tables.UserRoomKeys, error) {
	s := &userRoomKeysStatements{}
	return s, sqlutil.StatementList{
		{&s.insertUserRoomPrivateKeyStmt, insertUserRoomPrivateKeySQL},
		{&s.insertUserRoomPublicKeyStmt, insertUserRoomPublicKeySQL},
		{&s.selectUserRoomKeyStmt, selectUserRoomKeySQL},
		{&s.selectUserNIDsStmt, selectUserNIDsSQL},
	}.Prepare(db)
}

func (s *userRoomKeysStatements) InsertUserRoomPrivatePublicKey(ctx context.Context, txn *sql.Tx, userNID types.EventStateKeyNID, roomNID types.RoomNID, key ed25519.PrivateKey) (result ed25519.PrivateKey, err error) {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.insertUserRoomPrivateKeyStmt)
	err = stmt.QueryRowContext(ctx, userNID, roomNID, key, key.Public()).Scan(&result)
	return result, err
}

func (s *userRoomKeysStatements) InsertUserRoomPublicKey(ctx context.Context, txn *sql.Tx, userNID types.EventStateKeyNID, roomNID types.RoomNID, key ed25519.PublicKey) (result ed25519.PublicKey, err error) {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.insertUserRoomPublicKeyStmt)
	err = stmt.QueryRowContext(ctx, userNID, roomNID, key).Scan(&result)
	return result, err
}

func (s *userRoomKeysStatements) SelectUserRoomPrivateKey(
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

func (s *userRoomKeysStatements) BulkSelectUserNIDs(ctx context.Context, txn *sql.Tx, senderKeys map[types.RoomNID][]ed25519.PublicKey) (map[string]types.UserRoomKeyPair, error) {
	stmt := sqlutil.TxStmtContext(ctx, txn, s.selectUserNIDsStmt)

	roomNIDs := make([]types.RoomNID, 0, len(senderKeys))
	var senders [][]byte
	for roomNID := range senderKeys {
		roomNIDs = append(roomNIDs, roomNID)
		for _, key := range senderKeys[roomNID] {
			senders = append(senders, key)
		}
	}
	rows, err := stmt.QueryContext(ctx, pq.Array(roomNIDs), pq.Array(senders))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "failed to close rows")

	result := make(map[string]types.UserRoomKeyPair, len(senders)+len(roomNIDs))
	var publicKey []byte
	userRoomKeyPair := types.UserRoomKeyPair{}
	for rows.Next() {
		if err = rows.Scan(&userRoomKeyPair.EventStateKeyNID, &userRoomKeyPair.RoomNID, &publicKey); err != nil {
			return nil, err
		}
		result[string(publicKey)] = userRoomKeyPair
	}
	return result, rows.Err()
}
