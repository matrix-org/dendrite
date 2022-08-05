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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/storage/postgres/deltas"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

var keyChangesSchema = `
-- Stores key change information about users. Used to determine when to send updated device lists to clients.
CREATE SEQUENCE IF NOT EXISTS keyserver_key_changes_seq;
CREATE TABLE IF NOT EXISTS keyserver_key_changes (
	change_id BIGINT PRIMARY KEY DEFAULT nextval('keyserver_key_changes_seq'),
    user_id TEXT NOT NULL,
    CONSTRAINT keyserver_key_changes_unique_per_user UNIQUE (user_id)
);
`

// Replace based on user ID. We don't care how many times the user's keys have changed, only that they
// have changed, hence we can just keep bumping the change ID for this user.
const upsertKeyChangeSQL = "" +
	"INSERT INTO keyserver_key_changes (user_id)" +
	" VALUES ($1)" +
	" ON CONFLICT ON CONSTRAINT keyserver_key_changes_unique_per_user" +
	" DO UPDATE SET change_id = nextval('keyserver_key_changes_seq')" +
	" RETURNING change_id"

const selectKeyChangesSQL = "" +
	"SELECT user_id, change_id FROM keyserver_key_changes WHERE change_id > $1 AND change_id <= $2"

type keyChangesStatements struct {
	db                   *sql.DB
	upsertKeyChangeStmt  *sql.Stmt
	selectKeyChangesStmt *sql.Stmt
}

func NewPostgresKeyChangesTable(db *sql.DB) (tables.KeyChanges, error) {
	s := &keyChangesStatements{
		db: db,
	}
	_, err := db.Exec(keyChangesSchema)
	if err != nil {
		return s, err
	}

	// TODO: Remove when we are sure we are not having goose artefacts in the db
	// This forces an error, which indicates the migration is already applied, since the
	// column partition was removed from the table
	err = db.QueryRow("SELECT partition FROM keyserver_key_changes LIMIT 1;").Scan()
	if err == nil {
		m := sqlutil.NewMigrator(db)
		m.AddMigrations(sqlutil.Migration{
			Version: "keyserver: refactor key changes",
			Up:      deltas.UpRefactorKeyChanges,
		})
		return s, m.Up(context.Background())
	}
	return s, nil
}

func (s *keyChangesStatements) Prepare() (err error) {
	if s.upsertKeyChangeStmt, err = s.db.Prepare(upsertKeyChangeSQL); err != nil {
		return err
	}
	if s.selectKeyChangesStmt, err = s.db.Prepare(selectKeyChangesSQL); err != nil {
		return err
	}
	return nil
}

func (s *keyChangesStatements) InsertKeyChange(ctx context.Context, userID string) (changeID int64, err error) {
	err = s.upsertKeyChangeStmt.QueryRowContext(ctx, userID).Scan(&changeID)
	return
}

func (s *keyChangesStatements) SelectKeyChanges(
	ctx context.Context, fromOffset, toOffset int64,
) (userIDs []string, latestOffset int64, err error) {
	latestOffset = fromOffset
	rows, err := s.selectKeyChangesStmt.QueryContext(ctx, fromOffset, toOffset)
	if err != nil {
		return nil, 0, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectKeyChangesStmt: rows.close() failed")
	for rows.Next() {
		var userID string
		var offset int64
		if err := rows.Scan(&userID, &offset); err != nil {
			return nil, 0, err
		}
		if offset > latestOffset {
			latestOffset = offset
		}
		userIDs = append(userIDs, userID)
	}
	return
}
