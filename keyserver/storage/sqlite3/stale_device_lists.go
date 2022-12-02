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
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

var staleDeviceListsSchema = `
-- Stores whether a user's device lists are stale or not.
CREATE TABLE IF NOT EXISTS keyserver_stale_device_lists (
    user_id TEXT PRIMARY KEY NOT NULL,
	domain TEXT NOT NULL,
	is_stale BOOLEAN NOT NULL,
	ts_added_secs BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS keyserver_stale_device_lists_idx ON keyserver_stale_device_lists (domain, is_stale);
`

const upsertStaleDeviceListSQL = "" +
	"INSERT INTO keyserver_stale_device_lists (user_id, domain, is_stale, ts_added_secs)" +
	" VALUES ($1, $2, $3, $4)" +
	" ON CONFLICT (user_id)" +
	" DO UPDATE SET is_stale = $3, ts_added_secs = $4"

const selectStaleDeviceListsWithDomainsSQL = "" +
	"SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1 AND domain = $2 ORDER BY ts_added_secs DESC"

const selectStaleDeviceListsSQL = "" +
	"SELECT user_id FROM keyserver_stale_device_lists WHERE is_stale = $1 ORDER BY ts_added_secs DESC"

type staleDeviceListsStatements struct {
	db                                    *sql.DB
	upsertStaleDeviceListStmt             *sql.Stmt
	selectStaleDeviceListsWithDomainsStmt *sql.Stmt
	selectStaleDeviceListsStmt            *sql.Stmt
}

func NewSqliteStaleDeviceListsTable(db *sql.DB) (tables.StaleDeviceLists, error) {
	s := &staleDeviceListsStatements{
		db: db,
	}
	_, err := db.Exec(staleDeviceListsSchema)
	if err != nil {
		return nil, err
	}
	if s.upsertStaleDeviceListStmt, err = db.Prepare(upsertStaleDeviceListSQL); err != nil {
		return nil, err
	}
	if s.selectStaleDeviceListsStmt, err = db.Prepare(selectStaleDeviceListsSQL); err != nil {
		return nil, err
	}
	if s.selectStaleDeviceListsWithDomainsStmt, err = db.Prepare(selectStaleDeviceListsWithDomainsSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *staleDeviceListsStatements) InsertStaleDeviceList(ctx context.Context, userID string, isStale bool) error {
	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return err
	}
	_, err = s.upsertStaleDeviceListStmt.ExecContext(ctx, userID, string(domain), isStale, gomatrixserverlib.AsTimestamp(time.Now()))
	return err
}

func (s *staleDeviceListsStatements) SelectUserIDsWithStaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error) {
	// we only query for 1 domain or all domains so optimise for those use cases
	if len(domains) == 0 {
		rows, err := s.selectStaleDeviceListsStmt.QueryContext(ctx, true)
		if err != nil {
			return nil, err
		}
		return rowsToUserIDs(ctx, rows)
	}
	var result []string
	for _, domain := range domains {
		rows, err := s.selectStaleDeviceListsWithDomainsStmt.QueryContext(ctx, true, string(domain))
		if err != nil {
			return nil, err
		}
		userIDs, err := rowsToUserIDs(ctx, rows)
		if err != nil {
			return nil, err
		}
		result = append(result, userIDs...)
	}
	return result, nil
}

func rowsToUserIDs(ctx context.Context, rows *sql.Rows) (result []string, err error) {
	defer internal.CloseAndLogIfError(ctx, rows, "closing rowsToUserIDs failed")
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		result = append(result, userID)
	}
	return result, rows.Err()
}
