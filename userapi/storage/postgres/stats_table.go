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

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

const userDailyVisitsSchema = `
CREATE TABLE IF NOT EXISTS user_daily_visits (
    localpart TEXT NOT NULL,
	device_id TEXT NOT NULL,
	timestamp BIGINT NOT NULL,
	user_agent TEXT
);

-- Device IDs and timestamp must be unique for a given user per day
CREATE UNIQUE INDEX IF NOT EXISTS localpart_device_timestamp_idx ON user_daily_visits(localpart, device_id, timestamp);
CREATE INDEX IF NOT EXISTS timestamp_idx ON user_daily_visits(timestamp);
CREATE INDEX IF NOT EXISTS localpart_timestamp_idx ON user_daily_visits(localpart, timestamp);
`

const countUsersLastSeenAfterSQL = "" +
	"SELECT COUNT(*) FROM (" +
	" SELECT user_id FROM device_devices WHERE last_seen > $1 " +
	" GROUP BY user_id" +
	" )"

const countR30UsersSQL = `
SELECT platform, COUNT(*) FROM (
	SELECT users.localpart, platform, users.created_ts, MAX(uip.last_seen_ts)
	FROM account_accounts users
	INNER JOIN
	(SELECT 
		localpart, last_seen_ts,
		CASE
	    	WHEN user_agent LIKE '%%Android%%' OR display_name LIKE '%%android%%' THEN 'android'
    	    WHEN user_agent LIKE '%%iOS%%' THEN 'ios'
        	WHEN user_agent LIKE '%%Electron%%' THEN 'electron'
        	WHEN user_agent LIKE '%%Mozilla%%' THEN 'web'
        	WHEN user_agent LIKE '%%Gecko%%' THEN 'web'
        	ELSE 'unknown'
		END
    	AS platform
		FROM device_devices
	) uip
	ON users.localpart = uip.localpart
	AND users.account_type <> 4
	AND users.created_ts < $1
	AND uip.last_seen_ts > $1,
	AND (uip.last_seen_ts) - users.created_ts > $2
	GROUP BY users.localpart, platform, users.created_ts
	) u GROUP BY PLATFORM
`

const countR30UsersV2SQL = `

`

const insertUserDailyVists = `
	INSERT INTO user_daily_visits(localpart, device_id, timestamp, user_agent) VALUES ($1, $2, $3, $4)
	ON CONFLICT ON CONSTRAINT localpart_device_timestamp_idx DO NOTHING
`

type statsStatements struct {
	serverName                  gomatrixserverlib.ServerName
	countUsersLastSeenAfterStmt *sql.Stmt
	countR30UsersStmt           *sql.Stmt
	countR30UsersV2Stmt         *sql.Stmt
	insertUserDailyVisits       *sql.Stmt
}

func NewPostgresStatsTable(db *sql.DB, serverName gomatrixserverlib.ServerName) (tables.StatsTable, error) {
	s := &statsStatements{
		serverName: serverName,
	}
	_, err := db.Exec(userDailyVisitsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.countUsersLastSeenAfterStmt, countUsersLastSeenAfterSQL},
		{&s.countR30UsersStmt, countR30UsersSQL},
		{&s.countR30UsersV2Stmt, countR30UsersV2SQL},
		{&s.insertUserDailyVisits, insertUserDailyVists},
	}.Prepare(db)
}

func (s *statsStatements) DailyUsers(ctx context.Context, txn *sql.Tx) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countUsersLastSeenAfterStmt)
	lastSeenAfter := time.Now().AddDate(0, 0, -1)
	err = stmt.QueryRowContext(ctx,
		gomatrixserverlib.AsTimestamp(lastSeenAfter),
	).Scan(&result)
	return
}

func (s *statsStatements) MonthlyUsers(ctx context.Context, txn *sql.Tx) (result int64, err error) {
	stmt := sqlutil.TxStmt(txn, s.countUsersLastSeenAfterStmt)
	lastSeenAfter := time.Now().AddDate(0, 0, -30)
	err = stmt.QueryRowContext(ctx,
		gomatrixserverlib.AsTimestamp(lastSeenAfter),
	).Scan(&result)
	return
}

/* R30Users counts the number of 30 day retained users, defined as:
- Users who have created their accounts more than 30 days ago
- Where last seen at most 30 days ago
- Where account creation and last_seen are > 30 days apart
*/
func (s *statsStatements) R30Users(ctx context.Context, txn *sql.Tx) (map[string]int64, error) {
	stmt := sqlutil.TxStmt(txn, s.countR30UsersStmt)
	lastSeenAfter := time.Now().AddDate(0, 0, -30)
	diff := time.Hour * 24 * 30

	rows, err := stmt.QueryContext(ctx,
		gomatrixserverlib.AsTimestamp(lastSeenAfter),
		diff.Milliseconds(),
	)
	if err != nil {
		return nil, err
	}

	var platform string
	var count int64
	var result = make(map[string]int64)
	for rows.Next() {
		if err := rows.Scan(&platform, &count); err != nil {
			return nil, err
		}
		result["all"] += count
		if platform == "unknown" {
			continue
		}
		result[platform] = count
	}

	return map[string]int64{}, rows.Err()
}

/* R30UsersV2 counts the number of 30 day retained users, defined as users that:
- Appear more than once in the past 60 days
- Have more than 30 days between the most and least recent appearances that occurred in the past 60 days.
*/
func (s *statsStatements) R30UsersV2(ctx context.Context, txn *sql.Tx) (map[string]int64, error) {
	stmt := sqlutil.TxStmt(txn, s.countR30UsersV2Stmt)
	lastSeenAfter := time.Now().AddDate(0, 0, -30)
	diff := time.Hour * 24 * 30
	diff.Milliseconds()

	rows, err := stmt.QueryContext(ctx,
		gomatrixserverlib.AsTimestamp(lastSeenAfter),
		diff.Milliseconds(),
	)
	if err != nil {
		return nil, err
	}

	var platform string
	var count int64
	var result = make(map[string]int64)
	for rows.Next() {
		if err := rows.Scan(&platform, &count); err != nil {
			return nil, err
		}
		result["all"] += count
		if platform == "unknown" {
			continue
		}
		result[platform] = count
	}

	return map[string]int64{}, rows.Err()
}

func (s *statsStatements) InsertUserDailyVisits(ctx context.Context, txn *sql.Tx, localpart, deviceID string, timestamp int64, userAgent string) error {
	stmt := sqlutil.TxStmt(txn, s.insertUserDailyVisits)
	_, err := stmt.ExecContext(ctx, localpart, deviceID, timestamp, userAgent)
	return err
}
