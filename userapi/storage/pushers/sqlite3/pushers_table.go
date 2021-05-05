// Copyright 2021 Dan Peleg <dan@globekeeper.com>
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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const pushersSchema = `
-- Stores data about pushers.
CREATE TABLE IF NOT EXISTS pusher_pushers (
	localpart TEXT,
	session_id BIGINT,
	profile_tag TEXT,
	kind TEXT,
	app_id VARCHAR(64),
	app_display_name TEXT,
	device_display_name TEXT,
	pushkey VARCHAR(512),
	lang TEXT,
	data TEXT,

	UNIQUE (app_id, pushkey, localpart)
);
`
const insertPusherSQL = "" +
	"INSERT INTO pusher_pushers (localpart, session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)"

const selectPushersByLocalpartSQL = "" +
	"SELECT session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data FROM pusher_pushers WHERE localpart = $1"

const selectPusherByPushkeySQL = "" +
	"SELECT session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data FROM pusher_pushers WHERE localpart = $1 AND pushkey = $2"

const updatePusherSQL = "" +
	"UPDATE pusher_pushers SET kind = $1, app_id = $2, app_display_name = $3, device_display_name = $4, profile_tag = $5, lang = $6, data = $7 WHERE localpart = $8 AND pushkey = $9"

const deletePusherSQL = "" +
	"DELETE FROM pusher_pushers WHERE app_id = $1 AND pushkey = $2 AND localpart = $3"

type pushersStatements struct {
	db                           *sql.DB
	writer                       sqlutil.Writer
	insertPusherStmt             *sql.Stmt
	selectPushersByLocalpartStmt *sql.Stmt
	selectPusherByPushkeyStmt    *sql.Stmt
	updatePusherStmt             *sql.Stmt
	deletePusherStmt             *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
}

func (s *pushersStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(pushersSchema)
	return err
}

func (s *pushersStatements) prepare(db *sql.DB, writer sqlutil.Writer, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.writer = writer
	if s.insertPusherStmt, err = db.Prepare(insertPusherSQL); err != nil {
		return
	}
	if s.selectPushersByLocalpartStmt, err = db.Prepare(selectPushersByLocalpartSQL); err != nil {
		return
	}
	if s.selectPusherByPushkeyStmt, err = db.Prepare(selectPusherByPushkeySQL); err != nil {
		return
	}
	if s.updatePusherStmt, err = db.Prepare(updatePusherSQL); err != nil {
		return
	}
	if s.deletePusherStmt, err = db.Prepare(deletePusherSQL); err != nil {
		return
	}
	s.serverName = server
	return
}

// insertPusher creates a new pusher.
// Returns an error if the user already has a pusher with the given pusher pushkey.
// Returns nil error success.
func (s *pushersStatements) insertPusher(
	ctx context.Context, txn *sql.Tx, session_id int64,
	pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertPusherStmt)
	_, err := stmt.ExecContext(ctx, localpart, session_id, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data)
	logrus.Debugf("🥳 Created pusher %d", session_id)
	return err
}

func (s *pushersStatements) selectPushersByLocalpart(
	ctx context.Context, txn *sql.Tx, localpart string,
) ([]api.Pusher, error) {
	pushers := []api.Pusher{}
	rows, err := sqlutil.TxStmt(txn, s.selectPushersByLocalpartStmt).QueryContext(ctx, localpart)

	if err != nil {
		return pushers, err
	}

	for rows.Next() {
		logrus.Debug("Next pusher row...")
		var pusher api.Pusher
		var sessionid sql.NullInt64
		var pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data sql.NullString
		err = rows.Scan(&sessionid, &pushkey, &kind, &appid, &appdisplayname, &devicedisplayname, &profiletag, &lang, &data)
		if err != nil {
			return pushers, err
		}
		if sessionid.Valid {
			pusher.SessionID = sessionid.Int64
		}
		if pushkey.Valid {
			pusher.PushKey = pushkey.String
		}
		if kind.Valid {
			pusher.Kind = kind.String
		}
		if appid.Valid {
			pusher.AppID = appid.String
		}
		if appdisplayname.Valid {
			pusher.AppDisplayName = appdisplayname.String
		}
		if devicedisplayname.Valid {
			pusher.DeviceDisplayName = devicedisplayname.String
		}
		if profiletag.Valid {
			pusher.ProfileTag = profiletag.String
		}
		if lang.Valid {
			pusher.Language = lang.String
		}
		if data.Valid {
			pusher.Data = data.String
		}

		pusher.UserID = userutil.MakeUserID(localpart, s.serverName)
		pushers = append(pushers, pusher)
	}

	logrus.Debugf("🤓 Database returned %d pushers", len(pushers))
	return pushers, nil
}

// selectPusherByID retrieves a pusher from the database with the given user
// localpart and pusherID
func (s *pushersStatements) selectPusherByPushkey(
	ctx context.Context, localpart, pushkey string,
) (*api.Pusher, error) {
	var pusher api.Pusher
	var id, key, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data sql.NullString
	stmt := s.selectPusherByPushkeyStmt
	err := stmt.QueryRowContext(ctx, localpart, pushkey).Scan(&id, &key, &kind, &appid, &appdisplayname, &devicedisplayname, &profiletag, &lang, &data)
	if err == nil {
		pusher.UserID = userutil.MakeUserID(localpart, s.serverName)
		if key.Valid {
			pusher.PushKey = key.String
		}
		if kind.Valid {
			pusher.Kind = kind.String
		}
		if appid.Valid {
			pusher.AppID = appid.String
		}
		if appdisplayname.Valid {
			pusher.AppDisplayName = appdisplayname.String
		}
		if devicedisplayname.Valid {
			pusher.DeviceDisplayName = devicedisplayname.String
		}
		if profiletag.Valid {
			pusher.ProfileTag = profiletag.String
		}
		if lang.Valid {
			pusher.Language = lang.String
		}
		if data.Valid {
			pusher.Data = data.String
		}
	}
	return &pusher, err
}

func (s *pushersStatements) updatePusher(
	ctx context.Context, txn *sql.Tx, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
) error {
	stmt := sqlutil.TxStmt(txn, s.updatePusherStmt)
	_, err := stmt.ExecContext(ctx, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart, pushkey)
	return err
}

func (s *pushersStatements) deletePusher(
	ctx context.Context, txn *sql.Tx, appid, pushkey, localpart string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deletePusherStmt)
	_, err := stmt.ExecContext(ctx, appid, pushkey, localpart)
	return err
}
