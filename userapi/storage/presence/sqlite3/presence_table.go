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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/types"
)

const presenceSchema = `
-- Stores data about presence
CREATE TABLE IF NOT EXISTS presence_presences (
	-- The ID
	id INTEGER PRIMARY KEY AUTOINCREMENT,
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
CREATE INDEX IF NOT EXISTS presence_presences_id ON presence_presences(id);
CREATE INDEX IF NOT EXISTS presence_presences_user_id ON presence_presences(user_id);
`

const upsertPresenceSQL = "" +
	"INSERT INTO presence_presences" +
	" (user_id, presence, status_msg, last_active_ts)" +
	" VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT (user_id)" +
	" DO UPDATE SET id = rowid+1," +
	" presence = $5, status_msg = $6, last_active_ts = $7" +
	" RETURNING id"

const selectPresenceForUserSQL = "" +
	"SELECT presence, status_msg, last_active_ts" +
	" FROM presence_presences" +
	" WHERE user_id = $1 LIMIT 1"

type presenceStatements struct {
	upsertPresenceStmt         *sql.Stmt
	selectPresenceForUsersStmt *sql.Stmt
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
	err = stmt.QueryRowContext(ctx, userID, presence, statusMsg, lastActiveTS, presence, statusMsg, lastActiveTS).Scan(&pos)
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
