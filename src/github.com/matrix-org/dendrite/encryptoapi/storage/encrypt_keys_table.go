// Copyright 2018 Vector Creations Ltd
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

package storage

import (
	"database/sql"
	"context"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/encryptoapi/types"
	"github.com/lib/pq"
)

const keysSchema = `
-- The media_repository table holds metadata for each media file stored and accessible to the local server,
-- the actual file is stored separately.
CREATE TABLE IF NOT EXISTS encrypt_keys (
    device_id TEXT 				NOT NULL,
    user_id TEXT 				NOT NULL,
    key_id TEXT 						,
    key_type TEXT 				NOT NULL,
    key_info TEXT 				NOT NULL,
    algorithm TEXT 				NOT NULL,
    signature TEXT 				NOT NULL
);
`
const insertkeySQL = `
INSERT INTO encrypt_keys (device_id, user_id, key_id, key_type, key_info, algorithm, signature)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`
const selectkeySQL = `
SELECT user_id, device_id, key_id, key_type, key_info, algorithm, signature FROM encrypt_keys 
WHERE user_id = $1 AND device_id = $2
`
const deleteSinglekeySQL = `
SELECT user_id, device_id, key_id, key_type, key_info, algorithm, signature FROM encrypt_keys 
WHERE user_id = $1 AND device_id = $2 AND algorithm = $3
`
const selectSinglekeySQL = `
DELETE FROM encrypt_keys 
WHERE user_id = $1 AND device_id = $2 AND algorithm = $3 AND key_id = $4
`
const selectInkeysSQL = `
SELECT user_id, device_id, key_id, key_type, key_info, algorithm, signature FROM encrypt_keys
 WHERE user_id = $1 AND key_type = 'device_key' AND device_id = ANY($2)
`
const selectAllkeysSQL = `
SELECT user_id, device_id, key_id, key_type, key_info, algorithm, signature FROM encrypt_keys 
WHERE user_id = $1 AND key_type = $2
`

type keyStatements struct {
	insertKeyStmt       *sql.Stmt
	selectKeyStmt       *sql.Stmt
	selectInKeysStmt    *sql.Stmt
	selectAllKeyStmt    *sql.Stmt
	selectSingleKeyStmt *sql.Stmt
	deleteSingleKeyStmt *sql.Stmt
}

func (s *keyStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(keysSchema)
	if err != nil {
		return
	}
	if s.insertKeyStmt, err = db.Prepare(insertkeySQL); err != nil {
		return
	}
	if s.selectKeyStmt, err = db.Prepare(selectkeySQL); err != nil {
		return
	}
	if s.selectInKeysStmt, err = db.Prepare(selectInkeysSQL); err != nil {
		return
	}
	if s.selectAllKeyStmt, err = db.Prepare(selectAllkeysSQL); err != nil {
		return
	}
	if s.deleteSingleKeyStmt, err = db.Prepare(selectSinglekeySQL); err != nil {
		return
	}
	if s.selectSingleKeyStmt, err = db.Prepare(deleteSinglekeySQL); err != nil {
		return
	}
	return
}

func (ks *keyStatements) insertKey(
	ctx context.Context, txn *sql.Tx,
	deviceID, userID, keyID, keyTyp, keyInfo, algorithm, signature string,
) error {
	stmt := common.TxStmt(txn, ks.insertKeyStmt)
	_, err := stmt.ExecContext(ctx, deviceID, userID, keyID, keyTyp, keyInfo, algorithm, signature)
	return err
}

func (ks *keyStatements) selectKey(
	ctx context.Context,
	txn *sql.Tx,
	deviceID, userID string,
) (holders []types.KeyHolder, err error) {
	stmt := common.TxStmt(txn, ks.selectKeyStmt)
	rows, err := stmt.QueryContext(ctx, userID, deviceID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		single := &types.KeyHolder{}
		if err := rows.Scan(
			&single.User_id,
			&single.Device_id,
			&single.Key_id,
			&single.Key_type,
			&single.Key,
			&single.Key_algorithm,
			&single.Signature,
		); err != nil {
			return nil, err
		}
		holders = append(holders, *single)
	}
	return holders, err
}
func (ks *keyStatements) selectSingleKey(
	ctx context.Context,
	userID, deviceID, algorithm string,
) (holder types.KeyHolder, err error) {
	stmt := ks.selectSingleKeyStmt
	row := stmt.QueryRowContext(ctx, userID, deviceID, algorithm)
	if err != nil {
		return holder, err
	}
	if err := row.Scan(
		&holder.User_id,
		&holder.Device_id,
		&holder.Key_id,
		&holder.Key_type,
		&holder.Key,
		&holder.Key_algorithm,
		&holder.Signature,
	); err != nil {
		deleteStmt := ks.deleteSingleKeyStmt
		deleteStmt.ExecContext(ctx, userID, deviceID, algorithm, holder.Key_id)
		return holder, err
	}
	return holder, err
}

func (ks *keyStatements) selectInKeys(
	ctx context.Context,
	userID string,
	arr []string,
) (holders []types.KeyHolder, err error) {
	rows := &sql.Rows{}
	stmt := ks.selectAllKeyStmt
	if len(arr) == 0 {
		rows, err = stmt.QueryContext(ctx, userID, "device_key")
	} else {
		stmt = ks.selectInKeysStmt
		list := pq.Array(arr)
		rows, err = stmt.QueryContext(ctx, userID, list)
	}
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		single := &types.KeyHolder{}
		if err := rows.Scan(
			&single.User_id,
			&single.Device_id,
			&single.Key_id,
			&single.Key_type,
			&single.Key,
			&single.Key_algorithm,
			&single.Signature,
		); err != nil {
			return nil, err
		}
		holders = append(holders, *single)
	}
	return holders, err
}
