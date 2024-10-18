// Copyright 2024 New Vector Ltd.
// Copyright 2021 Dan Peleg <dan@globekeeper.com>
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/element-hq/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// See https://matrix.org/docs/spec/client_server/r0.6.1#get-matrix-client-r0-pushers
const pushersSchema = `
CREATE TABLE IF NOT EXISTS userapi_pushers (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	-- The Matrix user ID localpart for this pusher
	localpart TEXT NOT NULL,
	server_name TEXT NOT NULL,
	session_id BIGINT DEFAULT NULL,
	profile_tag TEXT,
	kind TEXT NOT NULL,
	app_id TEXT NOT NULL,
	app_display_name TEXT NOT NULL,
	device_display_name TEXT NOT NULL,
	pushkey TEXT NOT NULL,
	pushkey_ts_ms BIGINT NOT NULL DEFAULT 0,
	lang TEXT NOT NULL,
	data TEXT NOT NULL
);

-- For faster deleting by app_id, pushkey pair.
CREATE INDEX IF NOT EXISTS userapi_pusher_app_id_pushkey_idx ON userapi_pushers(app_id, pushkey);

-- For faster retrieving by localpart.
CREATE INDEX IF NOT EXISTS userapi_pusher_localpart_idx ON userapi_pushers(localpart, server_name);

-- Pushkey must be unique for a given user and app.
CREATE UNIQUE INDEX IF NOT EXISTS userapi_pusher_app_id_pushkey_localpart_idx ON userapi_pushers(app_id, pushkey, localpart, server_name);
`

const insertPusherSQL = "" +
	"INSERT INTO userapi_pushers (localpart, server_name, session_id, pushkey, pushkey_ts_ms, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data)" +
	"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)" +
	"ON CONFLICT (app_id, pushkey, localpart, server_name) DO UPDATE SET session_id = $3, pushkey_ts_ms = $5, kind = $6, app_display_name = $8, device_display_name = $9, profile_tag = $10, lang = $11, data = $12"

const selectPushersSQL = "" +
	"SELECT session_id, pushkey, pushkey_ts_ms, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data FROM userapi_pushers WHERE localpart = $1 AND server_name = $2"

const deletePusherSQL = "" +
	"DELETE FROM userapi_pushers WHERE app_id = $1 AND pushkey = $2 AND localpart = $3 AND server_name = $4"

const deletePushersByAppIdAndPushKeySQL = "" +
	"DELETE FROM userapi_pushers WHERE app_id = $1 AND pushkey = $2"

func NewSQLitePusherTable(db *sql.DB) (tables.PusherTable, error) {
	s := &pushersStatements{}
	_, err := db.Exec(pushersSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertPusherStmt, insertPusherSQL},
		{&s.selectPushersStmt, selectPushersSQL},
		{&s.deletePusherStmt, deletePusherSQL},
		{&s.deletePushersByAppIdAndPushKeyStmt, deletePushersByAppIdAndPushKeySQL},
	}.Prepare(db)
}

type pushersStatements struct {
	insertPusherStmt                   *sql.Stmt
	selectPushersStmt                  *sql.Stmt
	deletePusherStmt                   *sql.Stmt
	deletePushersByAppIdAndPushKeyStmt *sql.Stmt
}

// insertPusher creates a new pusher.
// Returns an error if the user already has a pusher with the given pusher pushkey.
// Returns nil error success.
func (s *pushersStatements) InsertPusher(
	ctx context.Context, txn *sql.Tx, session_id int64,
	pushkey string, pushkeyTS int64, kind api.PusherKind, appid, appdisplayname, devicedisplayname, profiletag, lang, data,
	localpart string, serverName spec.ServerName,
) error {
	_, err := sqlutil.TxStmt(txn, s.insertPusherStmt).ExecContext(ctx, localpart, serverName, session_id, pushkey, pushkeyTS, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data)
	return err
}

func (s *pushersStatements) SelectPushers(
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName spec.ServerName,
) ([]api.Pusher, error) {
	pushers := []api.Pusher{}
	rows, err := s.selectPushersStmt.QueryContext(ctx, localpart, serverName)

	if err != nil {
		return pushers, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectPushers: rows.close() failed")

	for rows.Next() {
		var pusher api.Pusher
		var data []byte
		err = rows.Scan(
			&pusher.SessionID,
			&pusher.PushKey,
			&pusher.PushKeyTS,
			&pusher.Kind,
			&pusher.AppID,
			&pusher.AppDisplayName,
			&pusher.DeviceDisplayName,
			&pusher.ProfileTag,
			&pusher.Language,
			&data)
		if err != nil {
			return pushers, err
		}
		err := json.Unmarshal(data, &pusher.Data)
		if err != nil {
			return pushers, err
		}
		pushers = append(pushers, pusher)
	}

	logrus.Tracef("Database returned %d pushers", len(pushers))
	return pushers, rows.Err()
}

// deletePusher removes a single pusher by pushkey and user localpart.
func (s *pushersStatements) DeletePusher(
	ctx context.Context, txn *sql.Tx, appid, pushkey,
	localpart string, serverName spec.ServerName,
) error {
	_, err := sqlutil.TxStmt(txn, s.deletePusherStmt).ExecContext(ctx, appid, pushkey, localpart, serverName)
	return err
}

func (s *pushersStatements) DeletePushers(
	ctx context.Context, txn *sql.Tx, appid, pushkey string,
) error {
	_, err := sqlutil.TxStmt(txn, s.deletePushersByAppIdAndPushKeyStmt).ExecContext(ctx, appid, pushkey)
	return err
}
