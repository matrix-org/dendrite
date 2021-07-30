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

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	types2 "github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/userapi/types"
)

const presenceSchema = `
CREATE SEQUENCE IF NOT EXISTS presence_presence_id;

-- Stores data about presence
CREATE TABLE IF NOT EXISTS presence_presences (
	-- The ID
	id BIGINT PRIMARY KEY DEFAULT nextval('presence_presence_id'),
	-- The Matrix user ID
	user_id TEXT NOT NULL,
	-- The actual presence
	presence INT NOT NULL,
	-- The status message
	status_msg TEXT,
	-- The last time an action was received by this user
	last_active_ts BIGINT NOT NULL,
	CONSTRAINT presence_presences_unique UNIQUE (user_id)
);
CREATE INDEX IF NOT EXISTS presence_presences_user_id ON presence_presences(user_id);
`

const upsertPresenceSQL = "" +
	"INSERT INTO presence_presences" +
	" (user_id, presence, status_msg, last_active_ts)" +
	" VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT (user_id)" +
	" DO UPDATE SET id = currval('presence_presence_id')," +
	" presence = $2, status_msg = COALESCE($3, p.status_msg), last_active_ts = $4" +
	" RETURNING id"

const selectPresenceForUserSQL = "" +
	"SELECT presence, status_msg, last_active_ts" +
	" FROM presence_presences" +
	" WHERE user_id = $1 LIMIT 1"

const selectMaxPresenceSQL = "" +
	"SELECT MAX(id) FROM presence_presences"

type presenceStatements struct {
	upsertPresenceStmt         *sql.Stmt
	selectPresenceForUsersStmt *sql.Stmt
	selectMaxPresenceStmt      *sql.Stmt
}

func (p *presenceStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(presenceSchema)
	return err
}

func (p *presenceStatements) prepare(db *sql.DB) (err error) {
	if p.upsertPresenceStmt, err = db.Prepare(upsertPresenceSQL); err != nil {
		return
	}
	if p.selectPresenceForUsersStmt, err = db.Prepare(selectPresenceForUserSQL); err != nil {
		return
	}
	if p.selectMaxPresenceStmt, err = db.Prepare(selectMaxPresenceSQL); err != nil {
		return
	}
	return
}

// UpsertPresence creates/updates a presence status.
func (p *presenceStatements) UpsertPresence(
	ctx context.Context,
	txn *sql.Tx, userID,
	statusMsg string,
	presence types.PresenceStatus,
	lastActiveTS int64,
) (pos int64, err error) {
	stmt := sqlutil.TxStmt(txn, p.upsertPresenceStmt)
	msg := &statusMsg
	// avoid clearing status_msg when going idle
	// makes it impossible to delete status_msg, though..
	if statusMsg == "" {
		msg = nil
	}
	err = stmt.QueryRowContext(ctx, userID, presence, msg, lastActiveTS).Scan(&pos)
	return
}

// GetPresenceForUser returns the current presence of a user.
func (p *presenceStatements) GetPresenceForUser(
	ctx context.Context, txn *sql.Tx,
	userID string,
) (presence api.OutputPresenceData, err error) {
	presence.UserID = userID
	stmt := sqlutil.TxStmt(txn, p.selectPresenceForUsersStmt)

	err = stmt.QueryRowContext(ctx, userID).Scan(&presence.Presence, &presence.StatusMsg, &presence.LastActiveTS)
	return
}

func (p *presenceStatements) GetMaxPresenceID(ctx context.Context, txn *sql.Tx) (pos types2.StreamingToken, err error) {
	stmt := sqlutil.TxStmt(txn, p.selectMaxPresenceStmt)
	err = stmt.QueryRowContext(ctx).Scan(&pos)
	return
}
