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
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

var deviceKeysSchema = `
-- Stores device keys for users
CREATE TABLE IF NOT EXISTS keyserver_device_keys (
    user_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	ts_added_secs BIGINT NOT NULL,
	key_json TEXT NOT NULL,
	-- Clobber based on tuple of user/device.
    CONSTRAINT keyserver_device_keys_unique UNIQUE (user_id, device_id)
);
`

const upsertDeviceKeysSQL = "" +
	"INSERT INTO keyserver_device_keys (user_id, device_id, ts_added_secs, key_json)" +
	" VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT ON CONSTRAINT keyserver_device_keys_unique" +
	" DO UPDATE SET key_json = $4"

const selectDeviceKeysSQL = "" +
	"SELECT key_json FROM keyserver_device_keys WHERE user_id=$1 AND device_id=$2"

const selectBatchDeviceKeysSQL = "" +
	"SELECT device_id, key_json FROM keyserver_device_keys WHERE user_id=$1"

type deviceKeysStatements struct {
	db                        *sql.DB
	upsertDeviceKeysStmt      *sql.Stmt
	selectDeviceKeysStmt      *sql.Stmt
	selectBatchDeviceKeysStmt *sql.Stmt
}

func NewPostgresDeviceKeysTable(db *sql.DB) (tables.DeviceKeys, error) {
	s := &deviceKeysStatements{
		db: db,
	}
	_, err := db.Exec(deviceKeysSchema)
	if err != nil {
		return nil, err
	}
	if s.upsertDeviceKeysStmt, err = db.Prepare(upsertDeviceKeysSQL); err != nil {
		return nil, err
	}
	if s.selectDeviceKeysStmt, err = db.Prepare(selectDeviceKeysSQL); err != nil {
		return nil, err
	}
	if s.selectBatchDeviceKeysStmt, err = db.Prepare(selectBatchDeviceKeysSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *deviceKeysStatements) SelectDeviceKeysJSON(ctx context.Context, keys []api.DeviceKeys) error {
	for i, key := range keys {
		var keyJSONStr string
		err := s.selectDeviceKeysStmt.QueryRowContext(ctx, key.UserID, key.DeviceID).Scan(&keyJSONStr)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		// this will be '' when there is no device
		keys[i].KeyJSON = []byte(keyJSONStr)
	}
	return nil
}

func (s *deviceKeysStatements) InsertDeviceKeys(ctx context.Context, keys []api.DeviceKeys) error {
	now := time.Now().Unix()
	return sqlutil.WithTransaction(s.db, func(txn *sql.Tx) error {
		for _, key := range keys {
			_, err := txn.Stmt(s.upsertDeviceKeysStmt).ExecContext(
				ctx, key.UserID, key.DeviceID, now, string(key.KeyJSON),
			)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *deviceKeysStatements) SelectBatchDeviceKeys(ctx context.Context, userID string, deviceIDs []string) ([]api.DeviceKeys, error) {
	rows, err := s.selectBatchDeviceKeysStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectBatchDeviceKeysStmt: rows.close() failed")
	deviceIDMap := make(map[string]bool)
	for _, d := range deviceIDs {
		deviceIDMap[d] = true
	}
	var result []api.DeviceKeys
	for rows.Next() {
		var dk api.DeviceKeys
		dk.UserID = userID
		var keyJSON string
		if err := rows.Scan(&dk.DeviceID, &keyJSON); err != nil {
			return nil, err
		}
		dk.KeyJSON = []byte(keyJSON)
		// include the key if we want all keys (no device) or it was asked
		if deviceIDMap[dk.DeviceID] || len(deviceIDs) == 0 {
			result = append(result, dk)
		}
	}
	return result, rows.Err()
}
