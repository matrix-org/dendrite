// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/element-hq/dendrite/userapi/storage/tables"
)

var deviceKeysSchema = `
-- Stores device keys for users
CREATE TABLE IF NOT EXISTS keyserver_device_keys (
    user_id TEXT NOT NULL,
	device_id TEXT NOT NULL,
	ts_added_secs BIGINT NOT NULL,
	key_json TEXT NOT NULL,
	-- the stream ID of this key, scoped per-user. This gets updated when the device key changes.
	-- This means we do not store an unbounded append-only log of device keys, which is not actually
	-- required in the spec because in the event of a missed update the server fetches the entire
	-- current set of keys rather than trying to 'fast-forward' or catchup missing stream IDs.
	stream_id BIGINT NOT NULL,
	display_name TEXT,
	-- Clobber based on tuple of user/device.
    CONSTRAINT keyserver_device_keys_unique UNIQUE (user_id, device_id)
);
`

const upsertDeviceKeysSQL = "" +
	"INSERT INTO keyserver_device_keys (user_id, device_id, ts_added_secs, key_json, stream_id, display_name)" +
	" VALUES ($1, $2, $3, $4, $5, $6)" +
	" ON CONFLICT ON CONSTRAINT keyserver_device_keys_unique" +
	" DO UPDATE SET key_json = $4, stream_id = $5, display_name = $6"

const selectDeviceKeysSQL = "" +
	"SELECT key_json, stream_id, display_name FROM keyserver_device_keys WHERE user_id=$1 AND device_id=$2"

const selectBatchDeviceKeysSQL = "" +
	"SELECT device_id, key_json, stream_id, display_name FROM keyserver_device_keys WHERE user_id=$1 AND key_json <> ''"

const selectBatchDeviceKeysWithEmptiesSQL = "" +
	"SELECT device_id, key_json, stream_id, display_name FROM keyserver_device_keys WHERE user_id=$1"

const selectMaxStreamForUserSQL = "" +
	"SELECT MAX(stream_id) FROM keyserver_device_keys WHERE user_id=$1"

const countStreamIDsForUserSQL = "" +
	"SELECT COUNT(*) FROM keyserver_device_keys WHERE user_id=$1 AND stream_id = ANY($2)"

const deleteDeviceKeysSQL = "" +
	"DELETE FROM keyserver_device_keys WHERE user_id=$1 AND device_id=$2"

const deleteAllDeviceKeysSQL = "" +
	"DELETE FROM keyserver_device_keys WHERE user_id=$1"

type deviceKeysStatements struct {
	db                                   *sql.DB
	upsertDeviceKeysStmt                 *sql.Stmt
	selectDeviceKeysStmt                 *sql.Stmt
	selectBatchDeviceKeysStmt            *sql.Stmt
	selectBatchDeviceKeysWithEmptiesStmt *sql.Stmt
	selectMaxStreamForUserStmt           *sql.Stmt
	countStreamIDsForUserStmt            *sql.Stmt
	deleteDeviceKeysStmt                 *sql.Stmt
	deleteAllDeviceKeysStmt              *sql.Stmt
}

func NewPostgresDeviceKeysTable(db *sql.DB) (tables.DeviceKeys, error) {
	s := &deviceKeysStatements{
		db: db,
	}
	_, err := db.Exec(deviceKeysSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.upsertDeviceKeysStmt, upsertDeviceKeysSQL},
		{&s.selectDeviceKeysStmt, selectDeviceKeysSQL},
		{&s.selectBatchDeviceKeysStmt, selectBatchDeviceKeysSQL},
		{&s.selectBatchDeviceKeysWithEmptiesStmt, selectBatchDeviceKeysWithEmptiesSQL},
		{&s.selectMaxStreamForUserStmt, selectMaxStreamForUserSQL},
		{&s.countStreamIDsForUserStmt, countStreamIDsForUserSQL},
		{&s.deleteDeviceKeysStmt, deleteDeviceKeysSQL},
		{&s.deleteAllDeviceKeysStmt, deleteAllDeviceKeysSQL},
	}.Prepare(db)
}

func (s *deviceKeysStatements) SelectDeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error {
	for i, key := range keys {
		var keyJSONStr string
		var streamID int64
		var displayName sql.NullString
		err := s.selectDeviceKeysStmt.QueryRowContext(ctx, key.UserID, key.DeviceID).Scan(&keyJSONStr, &streamID, &displayName)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		// this will be '' when there is no device
		keys[i].Type = api.TypeDeviceKeyUpdate
		keys[i].KeyJSON = []byte(keyJSONStr)
		keys[i].StreamID = streamID
		if displayName.Valid {
			keys[i].DisplayName = displayName.String
		}
	}
	return nil
}

func (s *deviceKeysStatements) SelectMaxStreamIDForUser(ctx context.Context, txn *sql.Tx, userID string) (streamID int64, err error) {
	// nullable if there are no results
	var nullStream sql.NullInt64
	err = sqlutil.TxStmt(txn, s.selectMaxStreamForUserStmt).QueryRowContext(ctx, userID).Scan(&nullStream)
	if err == sql.ErrNoRows {
		err = nil
	}
	if nullStream.Valid {
		streamID = nullStream.Int64
	}
	return
}

func (s *deviceKeysStatements) CountStreamIDsForUser(ctx context.Context, userID string, streamIDs []int64) (int, error) {
	// nullable if there are no results
	var count sql.NullInt32
	err := s.countStreamIDsForUserStmt.QueryRowContext(ctx, userID, pq.Int64Array(streamIDs)).Scan(&count)
	if err != nil {
		return 0, err
	}
	if count.Valid {
		return int(count.Int32), nil
	}
	return 0, nil
}

func (s *deviceKeysStatements) InsertDeviceKeys(ctx context.Context, txn *sql.Tx, keys []api.DeviceMessage) error {
	for _, key := range keys {
		now := time.Now().Unix()
		_, err := sqlutil.TxStmt(txn, s.upsertDeviceKeysStmt).ExecContext(
			ctx, key.UserID, key.DeviceID, now, string(key.KeyJSON), key.StreamID, key.DisplayName,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *deviceKeysStatements) DeleteDeviceKeys(ctx context.Context, txn *sql.Tx, userID, deviceID string) error {
	_, err := sqlutil.TxStmt(txn, s.deleteDeviceKeysStmt).ExecContext(ctx, userID, deviceID)
	return err
}

func (s *deviceKeysStatements) DeleteAllDeviceKeys(ctx context.Context, txn *sql.Tx, userID string) error {
	_, err := sqlutil.TxStmt(txn, s.deleteAllDeviceKeysStmt).ExecContext(ctx, userID)
	return err
}

func (s *deviceKeysStatements) SelectBatchDeviceKeys(ctx context.Context, userID string, deviceIDs []string, includeEmpty bool) ([]api.DeviceMessage, error) {
	var stmt *sql.Stmt
	if includeEmpty {
		stmt = s.selectBatchDeviceKeysWithEmptiesStmt
	} else {
		stmt = s.selectBatchDeviceKeysStmt
	}
	rows, err := stmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectBatchDeviceKeysStmt: rows.close() failed")
	deviceIDMap := make(map[string]bool)
	for _, d := range deviceIDs {
		deviceIDMap[d] = true
	}
	var result []api.DeviceMessage
	var displayName sql.NullString
	for rows.Next() {
		dk := api.DeviceMessage{
			Type: api.TypeDeviceKeyUpdate,
			DeviceKeys: &api.DeviceKeys{
				UserID: userID,
			},
		}
		if err := rows.Scan(&dk.DeviceID, &dk.KeyJSON, &dk.StreamID, &displayName); err != nil {
			return nil, err
		}
		if displayName.Valid {
			dk.DisplayName = displayName.String
		}
		// include the key if we want all keys (no device) or it was asked
		if deviceIDMap[dk.DeviceID] || len(deviceIDs) == 0 {
			result = append(result, dk)
		}
	}
	return result, rows.Err()
}
