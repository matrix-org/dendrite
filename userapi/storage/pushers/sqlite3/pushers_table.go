// Copyright 2017 Vector Creations Ltd
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

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const pushersSchema = `
-- This sequence is used for automatic allocation of session_id.
-- CREATE SEQUENCE IF NOT EXISTS pusher_session_id_seq START 1;

-- Stores data about pushers.
CREATE TABLE IF NOT EXISTS pusher_pushers (
    access_token TEXT PRIMARY KEY,
    session_id INTEGER,
    pusher_id TEXT ,
    localpart TEXT ,
    created_ts BIGINT,
    display_name TEXT,
    last_seen_ts BIGINT,
    ip TEXT,
    user_agent TEXT,

		UNIQUE (localpart, pusher_id)
);
`
const selectPushersByLocalpartSQL = "" +
	"SELECT pusher_id, display_name, last_seen_ts, ip, user_agent FROM pusher_pushers WHERE localpart = $1 AND pusher_id != $2"

type pushersStatements struct {
	db                           *sql.DB
	writer                       sqlutil.Writer
	selectPushersByLocalpartStmt *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
}

func (s *pushersStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(pushersSchema)
	return err
}

func (s *pushersStatements) prepare(db *sql.DB, writer sqlutil.Writer, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.writer = writer
	if s.selectPushersByLocalpartStmt, err = db.Prepare(selectPushersByLocalpartSQL); err != nil {
		return
	}
	s.serverName = server
	return
}

func (s *pushersStatements) selectPushersByLocalpart(
	ctx context.Context, txn *sql.Tx, localpart, exceptPusherID string,
) ([]api.Pusher, error) {
	pushers := []api.Pusher{}
	rows, err := sqlutil.TxStmt(txn, s.selectPushersByLocalpartStmt).QueryContext(ctx, localpart, exceptPusherID)

	if err != nil {
		return pushers, err
	}

	for rows.Next() {
		var dev api.Pusher
		var lastseents sql.NullInt64
		var id, displayname, ip, useragent sql.NullString
		err = rows.Scan(&id, &displayname, &lastseents, &ip, &useragent)
		if err != nil {
			return pushers, err
		}
		if id.Valid {
			dev.ID = id.String
		}

		dev.UserID = userutil.MakeUserID(localpart, s.serverName)
		pushers = append(pushers, dev)
	}

	return pushers, nil
}
