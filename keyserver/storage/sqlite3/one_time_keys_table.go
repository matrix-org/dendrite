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

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

var oneTimeKeysSchema = `
-- Stores one-time public keys for users
CREATE TABLE IF NOT EXISTS keyserver_one_time_keys (
    user_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	key_id TEXT NOT NULL,
	algorithm TEXT NOT NULL,
	ts_added_secs BIGINT NOT NULL,
	key_json TEXT NOT NULL,
	-- Clobber based on 4-uple of user/device/key/algorithm.
    UNIQUE (user_id, device_id, key_id, algorithm)
);

CREATE INDEX IF NOT EXISTS keyserver_one_time_keys_idx ON keyserver_one_time_keys (user_id, device_id);
`

const upsertKeysSQL = "" +
	"INSERT INTO keyserver_one_time_keys (user_id, device_id, key_id, algorithm, ts_added_secs, key_json)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT (user_id, device_id, key_id, algorithm)" +
	" DO UPDATE SET key_json = $6"

const selectKeysSQL = "" +
	"SELECT key_id, algorithm, key_json FROM keyserver_one_time_keys WHERE user_id=$1 AND device_id=$2"

const selectKeysCountSQL = "" +
	"SELECT algorithm, COUNT(key_id) FROM " +
	" (SELECT algorithm, key_id FROM keyserver_one_time_keys WHERE user_id = $1 AND device_id = $2 LIMIT 100)" +
	" x GROUP BY algorithm"

const deleteOneTimeKeySQL = "" +
	"DELETE FROM keyserver_one_time_keys WHERE user_id = $1 AND device_id = $2 AND algorithm = $3 AND key_id = $4"

const selectKeyByAlgorithmSQL = "" +
	"SELECT key_id, key_json FROM keyserver_one_time_keys WHERE user_id = $1 AND device_id = $2 AND algorithm = $3 LIMIT 1"

const deleteOneTimeKeysSQL = "" +
	"DELETE FROM keyserver_one_time_keys WHERE user_id = $1 AND device_id = $2"

type oneTimeKeysStatements struct {
	db                       *sql.DB
	upsertKeysStmt           *sql.Stmt
	selectKeysStmt           *sql.Stmt
	selectKeysCountStmt      *sql.Stmt
	selectKeyByAlgorithmStmt *sql.Stmt
	deleteOneTimeKeyStmt     *sql.Stmt
	deleteOneTimeKeysStmt    *sql.Stmt
}

func NewSqliteOneTimeKeysTable(db *sql.DB) (tables.OneTimeKeys, error) {
	s := &oneTimeKeysStatements{
		db: db,
	}
	_, err := db.Exec(oneTimeKeysSchema)
	if err != nil {
		return nil, err
	}
	if s.upsertKeysStmt, err = db.Prepare(upsertKeysSQL); err != nil {
		return nil, err
	}
	if s.selectKeysStmt, err = db.Prepare(selectKeysSQL); err != nil {
		return nil, err
	}
	if s.selectKeysCountStmt, err = db.Prepare(selectKeysCountSQL); err != nil {
		return nil, err
	}
	if s.selectKeyByAlgorithmStmt, err = db.Prepare(selectKeyByAlgorithmSQL); err != nil {
		return nil, err
	}
	if s.deleteOneTimeKeyStmt, err = db.Prepare(deleteOneTimeKeySQL); err != nil {
		return nil, err
	}
	if s.deleteOneTimeKeysStmt, err = db.Prepare(deleteOneTimeKeysSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *oneTimeKeysStatements) SelectOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error) {
	rows, err := s.selectKeysStmt.QueryContext(ctx, userID, deviceID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectKeysStmt: rows.close() failed")

	wantSet := make(map[string]bool, len(keyIDsWithAlgorithms))
	for _, ka := range keyIDsWithAlgorithms {
		wantSet[ka] = true
	}

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var keyID string
		var algorithm string
		var keyJSONStr string
		if err := rows.Scan(&keyID, &algorithm, &keyJSONStr); err != nil {
			return nil, err
		}
		keyIDWithAlgo := algorithm + ":" + keyID
		if wantSet[keyIDWithAlgo] {
			result[keyIDWithAlgo] = json.RawMessage(keyJSONStr)
		}
	}
	return result, rows.Err()
}

func (s *oneTimeKeysStatements) CountOneTimeKeys(ctx context.Context, userID, deviceID string) (*api.OneTimeKeysCount, error) {
	counts := &api.OneTimeKeysCount{
		DeviceID: deviceID,
		UserID:   userID,
		KeyCount: make(map[string]int),
	}
	rows, err := s.selectKeysCountStmt.QueryContext(ctx, userID, deviceID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectKeysCountStmt: rows.close() failed")
	for rows.Next() {
		var algorithm string
		var count int
		if err = rows.Scan(&algorithm, &count); err != nil {
			return nil, err
		}
		counts.KeyCount[algorithm] = count
	}
	return counts, nil
}

func (s *oneTimeKeysStatements) InsertOneTimeKeys(
	ctx context.Context, txn *sql.Tx, keys api.OneTimeKeys,
) (*api.OneTimeKeysCount, error) {
	now := time.Now().Unix()
	counts := &api.OneTimeKeysCount{
		DeviceID: keys.DeviceID,
		UserID:   keys.UserID,
		KeyCount: make(map[string]int),
	}
	for keyIDWithAlgo, keyJSON := range keys.KeyJSON {
		algo, keyID := keys.Split(keyIDWithAlgo)
		_, err := sqlutil.TxStmt(txn, s.upsertKeysStmt).ExecContext(
			ctx, keys.UserID, keys.DeviceID, keyID, algo, now, string(keyJSON),
		)
		if err != nil {
			return nil, err
		}
	}
	rows, err := sqlutil.TxStmt(txn, s.selectKeysCountStmt).QueryContext(ctx, keys.UserID, keys.DeviceID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectKeysCountStmt: rows.close() failed")
	for rows.Next() {
		var algorithm string
		var count int
		if err = rows.Scan(&algorithm, &count); err != nil {
			return nil, err
		}
		counts.KeyCount[algorithm] = count
	}

	return counts, rows.Err()
}

func (s *oneTimeKeysStatements) SelectAndDeleteOneTimeKey(
	ctx context.Context, txn *sql.Tx, userID, deviceID, algorithm string,
) (map[string]json.RawMessage, error) {
	var keyID string
	var keyJSON string
	err := sqlutil.TxStmtContext(ctx, txn, s.selectKeyByAlgorithmStmt).QueryRowContext(ctx, userID, deviceID, algorithm).Scan(&keyID, &keyJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_, err = sqlutil.TxStmtContext(ctx, txn, s.deleteOneTimeKeyStmt).ExecContext(ctx, userID, deviceID, algorithm, keyID)
	if err != nil {
		return nil, err
	}
	if keyJSON == "" {
		return nil, nil
	}
	return map[string]json.RawMessage{
		algorithm + ":" + keyID: json.RawMessage(keyJSON),
	}, err
}

func (s *oneTimeKeysStatements) DeleteOneTimeKeys(ctx context.Context, txn *sql.Tx, userID, deviceID string) error {
	_, err := sqlutil.TxStmt(txn, s.deleteOneTimeKeysStmt).ExecContext(ctx, userID, deviceID)
	return err
}
