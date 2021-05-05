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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

const pushersSchema = `
-- Stores data about pushers.
CREATE TABLE IF NOT EXISTS pusher_pushers (
	id SERIAL PRIMARY KEY,
	-- The Matrix user ID localpart for this pusher
	localpart TEXT NOT NULL,
	-- The Session ID used to create the Pusher
	session_id BIGINT DEFAULT NULL,
	-- This string determines which set of device specific rules this pusher executes.
	profile_tag TEXT NOT NULL,
	-- The kind of pusher. "http" is a pusher that sends HTTP pokes.
	kind TEXT,
	-- This is a reverse-DNS style identifier for the application. Max length, 64 chars.
	app_id VARCHAR(64),
	-- A string that will allow the user to identify what application owns this pusher.
	app_display_name TEXT,
	-- A string that will allow the user to identify what device owns this pusher.
	device_display_name TEXT,
	-- This is a unique identifier for this pusher. 
	-- The value you should use for this is the routing or destination address information for the notification, for example, 
	-- the APNS token for APNS or the Registration ID for GCM. If your notification client has no such concept, use any unique identifier. 
	-- If the kind is "email", this is the email address to send notifications to.
	-- Max length, 512 bytes.
	pushkey VARCHAR(512) NOT NULL,
	-- The preferred language for receiving notifications (e.g. 'en' or 'en-US')
	lang TEXT,
	-- A dictionary of information for the pusher implementation itself.
	data TEXT
);

-- Pushkey must be unique for a given user.
CREATE UNIQUE INDEX IF NOT EXISTS pusher_app_id_pushkey_localpart_idx ON pusher_pushers(app_id, pushkey, localpart);
`

const insertPusherSQL = "" +
	"INSERT INTO pusher_pushers(localpart, session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)"

const selectPushersByLocalpartSQL = "" +
	"SELECT session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data FROM pusher_pushers WHERE localpart = $1"

const selectPusherByPushkeySQL = "" +
	"SELECT session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data FROM pusher_pushers WHERE localpart = $1 AND pushkey = $2"

const updatePusherSQL = "" +
	"UPDATE pusher_pushers SET kind = $1, app_id = $2, app_display_name = $3, device_display_name = $4, profile_tag = $5, lang = $6, data = $7 WHERE localpart = $8 AND pushkey = $9"

const deletePusherSQL = "" +
	"DELETE FROM pusher_pushers WHERE app_id = $1 AND pushkey = $2 AND localpart = $3"

type pushersStatements struct {
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

func (s *pushersStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
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
	defer internal.CloseAndLogIfError(ctx, rows, "selectPushersByLocalpart: rows.close() failed")

	for rows.Next() {
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
	return pushers, rows.Err()
}

func (s *pushersStatements) selectPusherByPushkey(
	ctx context.Context, localpart, pushkey string,
) (*api.Pusher, error) {
	var pusher api.Pusher
	var sessionid sql.NullInt64
	var key, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data sql.NullString

	stmt := s.selectPusherByPushkeyStmt
	err := stmt.QueryRowContext(ctx, localpart, pushkey).Scan(&sessionid, &key, &kind, &appid, &appdisplayname, &devicedisplayname, &profiletag, &lang, &data)

	if err == nil {
		if sessionid.Valid {
			pusher.SessionID = sessionid.Int64
		}
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

		pusher.UserID = userutil.MakeUserID(localpart, s.serverName)
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

// deletePusher removes a single pusher by pushkey and user localpart.
func (s *pushersStatements) deletePusher(
	ctx context.Context, txn *sql.Tx, appid, pushkey, localpart string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deletePusherStmt)
	_, err := stmt.ExecContext(ctx, appid, pushkey, localpart)
	return err
}
