// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"encoding/json"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/userapi/api"
)

const keyBackupTableSchema = `
CREATE TABLE IF NOT EXISTS account_e2e_room_keys (
    user_id TEXT NOT NULL,
    room_id TEXT NOT NULL,
    session_id TEXT NOT NULL,

    version TEXT NOT NULL,
    first_message_index INTEGER NOT NULL,
    forwarded_count INTEGER NOT NULL,
    is_verified BOOLEAN NOT NULL,
    session_data TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS e2e_room_keys_idx ON account_e2e_room_keys(user_id, room_id, session_id);
`

const insertBackupKeySQL = "" +
	"INSERT INTO account_e2e_room_keys(user_id, room_id, session_id, version, first_message_index, forwarded_count, is_verified, session_data) " +
	"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"

const updateBackupKeySQL = "" +
	"UPDATE account_e2e_room_keys SET first_message_index=$1, forwarded_count=$2, is_verified=$3, session_data=$4 " +
	"WHERE user_id=$5 AND room_id=$6 AND session_id=$7"

const countKeysSQL = "" +
	"SELECT COUNT(*) FROM account_e2e_room_keys WHERE user_id = $1 AND version = $2"

const selectKeysSQL = "" +
	"SELECT room_id, session_id, first_message_index, forwarded_count, is_verified, session_data FROM account_e2e_room_keys " +
	"WHERE user_id = $1 AND version = $2"

type keyBackupStatements struct {
	insertBackupKeyStmt *sql.Stmt
	updateBackupKeyStmt *sql.Stmt
	countKeysStmt       *sql.Stmt
	selectKeysStmt      *sql.Stmt
}

func (s *keyBackupStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(keyBackupTableSchema)
	if err != nil {
		return
	}
	if s.insertBackupKeyStmt, err = db.Prepare(insertBackupKeySQL); err != nil {
		return
	}
	if s.updateBackupKeyStmt, err = db.Prepare(updateBackupKeySQL); err != nil {
		return
	}
	if s.countKeysStmt, err = db.Prepare(countKeysSQL); err != nil {
		return
	}
	if s.selectKeysStmt, err = db.Prepare(selectKeysSQL); err != nil {
		return
	}
	return
}

func (s keyBackupStatements) countKeys(
	ctx context.Context, txn *sql.Tx, userID, version string,
) (count int64, err error) {
	err = txn.Stmt(s.countKeysStmt).QueryRowContext(ctx, userID, version).Scan(&count)
	return
}

func (s *keyBackupStatements) insertBackupKey(
	ctx context.Context, txn *sql.Tx, userID, version string, key api.InternalKeyBackupSession,
) (err error) {
	_, err = txn.Stmt(s.insertBackupKeyStmt).ExecContext(
		ctx, userID, key.RoomID, key.SessionID, version, key.FirstMessageIndex, key.ForwardedCount, key.IsVerified, string(key.SessionData),
	)
	return
}

func (s *keyBackupStatements) updateBackupKey(
	ctx context.Context, txn *sql.Tx, userID, version string, key api.InternalKeyBackupSession,
) (err error) {
	_, err = txn.Stmt(s.updateBackupKeyStmt).ExecContext(
		ctx, key.FirstMessageIndex, key.ForwardedCount, key.IsVerified, string(key.SessionData), userID, key.RoomID, key.SessionID,
	)
	return
}

func (s *keyBackupStatements) selectKeys(
	ctx context.Context, txn *sql.Tx, userID, version string,
) (map[string]map[string]api.KeyBackupSession, error) {
	result := make(map[string]map[string]api.KeyBackupSession)
	rows, err := txn.Stmt(s.selectKeysStmt).QueryContext(ctx, userID, version)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectKeysStmt.Close failed")
	for rows.Next() {
		var key api.InternalKeyBackupSession
		// room_id, session_id, first_message_index, forwarded_count, is_verified, session_data
		var sessionDataStr string
		if err := rows.Scan(&key.RoomID, &key.SessionID, &key.FirstMessageIndex, &key.ForwardedCount, &key.IsVerified, &sessionDataStr); err != nil {
			return nil, err
		}
		key.SessionData = json.RawMessage(sessionDataStr)
		roomData := result[key.RoomID]
		if roomData == nil {
			roomData = make(map[string]api.KeyBackupSession)
		}
		roomData[key.SessionID] = key.KeyBackupSession
		result[key.RoomID] = roomData
	}
	return result, nil
}
