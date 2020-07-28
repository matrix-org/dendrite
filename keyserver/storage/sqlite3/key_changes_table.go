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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

var keyChangesSchema = `
-- Stores key change information about users. Used to determine when to send updated device lists to clients.
CREATE TABLE IF NOT EXISTS keyserver_key_changes (
	partition BIGINT NOT NULL,
	offset BIGINT NOT NULL,
	-- The key owner
	user_id TEXT NOT NULL,
	UNIQUE (partition, offset)
);
`

// Replace based on partition|offset - we should never insert duplicates unless the kafka logs are wiped.
// Rather than falling over, just overwrite (though this will mean clients with an existing sync token will
// miss out on updates). TODO: Ideally we would detect when kafka logs are purged then purge this table too.
const upsertKeyChangeSQL = "" +
	"INSERT INTO keyserver_key_changes (partition, offset, user_id)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT (partition, offset)" +
	" DO UPDATE SET user_id = $3"

// select the highest offset for each user in the range. The grouping by user gives distinct entries and then we just
// take the max offset value as the latest offset.
const selectKeyChangesSQL = "" +
	"SELECT user_id, MAX(offset) FROM keyserver_key_changes WHERE partition = $1 AND offset > $2 GROUP BY user_id"

type keyChangesStatements struct {
	db                   *sql.DB
	upsertKeyChangeStmt  *sql.Stmt
	selectKeyChangesStmt *sql.Stmt
}

func NewSqliteKeyChangesTable(db *sql.DB) (tables.KeyChanges, error) {
	s := &keyChangesStatements{
		db: db,
	}
	_, err := db.Exec(keyChangesSchema)
	if err != nil {
		return nil, err
	}
	if s.upsertKeyChangeStmt, err = db.Prepare(upsertKeyChangeSQL); err != nil {
		return nil, err
	}
	if s.selectKeyChangesStmt, err = db.Prepare(selectKeyChangesSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *keyChangesStatements) InsertKeyChange(ctx context.Context, partition int32, offset int64, userID string) error {
	_, err := s.upsertKeyChangeStmt.ExecContext(ctx, partition, offset, userID)
	return err
}

func (s *keyChangesStatements) SelectKeyChanges(
	ctx context.Context, partition int32, fromOffset int64,
) (userIDs []string, latestOffset int64, err error) {
	rows, err := s.selectKeyChangesStmt.QueryContext(ctx, partition, fromOffset)
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
