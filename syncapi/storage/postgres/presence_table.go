// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

const presenceSchema = `
CREATE SEQUENCE IF NOT EXISTS syncapi_presence_id;
-- Stores data about presence
CREATE TABLE IF NOT EXISTS syncapi_presence (
	-- The ID
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_presence_id'),
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
CREATE INDEX IF NOT EXISTS syncapi_presence_user_id ON syncapi_presence(user_id);
`

const upsertPresenceSQL = "" +
	"INSERT INTO syncapi_presence AS p" +
	" (user_id, presence, status_msg, last_active_ts)" +
	" VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT (user_id)" +
	" DO UPDATE SET id = nextval('syncapi_presence_id')," +
	" presence = $2, status_msg = COALESCE($3, p.status_msg), last_active_ts = $4" +
	" RETURNING id"

const upsertPresenceFromSyncSQL = "" +
	"INSERT INTO syncapi_presence AS p" +
	" (user_id, presence, last_active_ts)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT (user_id)" +
	" DO UPDATE SET id = nextval('syncapi_presence_id')," +
	" presence = $2, last_active_ts = $3" +
	" RETURNING id"

const selectPresenceForUserSQL = "" +
	"SELECT presence, status_msg, last_active_ts" +
	" FROM syncapi_presence" +
	" WHERE user_id = $1 LIMIT 1"

const selectMaxPresenceSQL = "" +
	"SELECT COALESCE(MAX(id), 0) FROM syncapi_presence"

const selectPresenceAfter = "" +
	" SELECT id, user_id, presence, status_msg, last_active_ts" +
	" FROM syncapi_presence" +
	" WHERE id > $1 AND last_active_ts >= $2" +
	" ORDER BY id ASC LIMIT $3"

type presenceStatements struct {
	upsertPresenceStmt         *sql.Stmt
	upsertPresenceFromSyncStmt *sql.Stmt
	selectPresenceForUsersStmt *sql.Stmt
	selectMaxPresenceStmt      *sql.Stmt
	selectPresenceAfterStmt    *sql.Stmt
}

func NewPostgresPresenceTable(db *sql.DB) (*presenceStatements, error) {
	_, err := db.Exec(presenceSchema)
	if err != nil {
		return nil, err
	}
	s := &presenceStatements{}
	return s, sqlutil.StatementList{
		{&s.upsertPresenceStmt, upsertPresenceSQL},
		{&s.upsertPresenceFromSyncStmt, upsertPresenceFromSyncSQL},
		{&s.selectPresenceForUsersStmt, selectPresenceForUserSQL},
		{&s.selectMaxPresenceStmt, selectMaxPresenceSQL},
		{&s.selectPresenceAfterStmt, selectPresenceAfter},
	}.Prepare(db)
}

// UpsertPresence creates/updates a presence status.
func (p *presenceStatements) UpsertPresence(
	ctx context.Context,
	txn *sql.Tx,
	userID string,
	statusMsg *string,
	presence types.Presence,
	lastActiveTS gomatrixserverlib.Timestamp,
	fromSync bool,
) (pos types.StreamPosition, err error) {
	if fromSync {
		stmt := sqlutil.TxStmt(txn, p.upsertPresenceFromSyncStmt)
		err = stmt.QueryRowContext(ctx, userID, presence, lastActiveTS).Scan(&pos)
	} else {
		stmt := sqlutil.TxStmt(txn, p.upsertPresenceStmt)
		err = stmt.QueryRowContext(ctx, userID, presence, statusMsg, lastActiveTS).Scan(&pos)
	}
	return
}

// GetPresenceForUser returns the current presence of a user.
func (p *presenceStatements) GetPresenceForUser(
	ctx context.Context, txn *sql.Tx,
	userID string,
) (*types.PresenceInternal, error) {
	result := &types.PresenceInternal{
		UserID: userID,
	}
	stmt := sqlutil.TxStmt(txn, p.selectPresenceForUsersStmt)
	err := stmt.QueryRowContext(ctx, userID).Scan(&result.Presence, &result.ClientFields.StatusMsg, &result.LastActiveTS)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	result.ClientFields.Presence = result.Presence.String()
	return result, err
}

func (p *presenceStatements) GetMaxPresenceID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error) {
	stmt := sqlutil.TxStmt(txn, p.selectMaxPresenceStmt)
	err = stmt.QueryRowContext(ctx).Scan(&pos)
	return
}

// GetPresenceAfter returns the changes presences after a given stream id
func (p *presenceStatements) GetPresenceAfter(
	ctx context.Context, txn *sql.Tx,
	after types.StreamPosition,
	filter gomatrixserverlib.EventFilter,
) (presences map[string]*types.PresenceInternal, err error) {
	presences = make(map[string]*types.PresenceInternal)
	stmt := sqlutil.TxStmt(txn, p.selectPresenceAfterStmt)
	afterTS := gomatrixserverlib.AsTimestamp(time.Now().Add(time.Minute * -5))
	rows, err := stmt.QueryContext(ctx, after, afterTS, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "GetPresenceAfter: failed to close rows")
	for rows.Next() {
		qryRes := &types.PresenceInternal{}
		if err := rows.Scan(&qryRes.StreamPos, &qryRes.UserID, &qryRes.Presence, &qryRes.ClientFields.StatusMsg, &qryRes.LastActiveTS); err != nil {
			return nil, err
		}
		qryRes.ClientFields.Presence = qryRes.Presence.String()
		presences[qryRes.UserID] = qryRes
	}
	return presences, rows.Err()
}
