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
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

func NewSqliteNotificationDataTable(db *sql.DB, streamID *StreamIDStatements) (tables.NotificationData, error) {
	_, err := db.Exec(notificationDataSchema)
	if err != nil {
		return nil, err
	}
	r := &notificationDataStatements{
		streamIDStatements: streamID,
	}
	return r, sqlutil.StatementList{
		{&r.upsertRoomUnreadCounts, upsertRoomUnreadNotificationCountsSQL},
		{&r.selectUserUnreadCounts, selectUserUnreadNotificationCountsSQL},
		{&r.selectMaxID, selectMaxNotificationIDSQL},
	}.Prepare(db)
}

type notificationDataStatements struct {
	streamIDStatements     *StreamIDStatements
	upsertRoomUnreadCounts *sql.Stmt
	selectUserUnreadCounts *sql.Stmt
	selectMaxID            *sql.Stmt
}

const notificationDataSchema = `
CREATE TABLE IF NOT EXISTS syncapi_notification_data (
	id INTEGER PRIMARY KEY,
	user_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	notification_count BIGINT NOT NULL DEFAULT 0,
	highlight_count BIGINT NOT NULL DEFAULT 0,
	CONSTRAINT syncapi_notifications_unique UNIQUE (user_id, room_id)
);`

const upsertRoomUnreadNotificationCountsSQL = `INSERT INTO syncapi_notification_data
  (user_id, room_id, notification_count, highlight_count)
  VALUES ($1, $2, $3, $4)
  ON CONFLICT (user_id, room_id)
  DO UPDATE SET id = $5, notification_count = $6, highlight_count = $7`

const selectUserUnreadNotificationCountsSQL = `SELECT
  id, room_id, notification_count, highlight_count
  FROM syncapi_notification_data
  WHERE
    user_id = $1 AND
    id BETWEEN $2 + 1 AND $3`

const selectMaxNotificationIDSQL = `SELECT CASE COUNT(*) WHEN 0 THEN 0 ELSE MAX(id) END FROM syncapi_notification_data`

func (r *notificationDataStatements) UpsertRoomUnreadCounts(ctx context.Context, txn *sql.Tx, userID, roomID string, notificationCount, highlightCount int) (pos types.StreamPosition, err error) {
	pos, err = r.streamIDStatements.nextNotificationID(ctx, nil)
	if err != nil {
		return
	}
	_, err = r.upsertRoomUnreadCounts.ExecContext(ctx, userID, roomID, notificationCount, highlightCount, pos, notificationCount, highlightCount)
	return
}

func (r *notificationDataStatements) SelectUserUnreadCounts(ctx context.Context, txn *sql.Tx, userID string, fromExcl, toIncl types.StreamPosition) (map[string]*eventutil.NotificationData, error) {
	rows, err := sqlutil.TxStmt(txn, r.selectUserUnreadCounts).QueryContext(ctx, userID, fromExcl, toIncl)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectUserUnreadCounts: rows.close() failed")

	roomCounts := map[string]*eventutil.NotificationData{}
	for rows.Next() {
		var id types.StreamPosition
		var roomID string
		var notificationCount, highlightCount int

		if err = rows.Scan(&id, &roomID, &notificationCount, &highlightCount); err != nil {
			return nil, err
		}

		roomCounts[roomID] = &eventutil.NotificationData{
			RoomID:                  roomID,
			UnreadNotificationCount: notificationCount,
			UnreadHighlightCount:    highlightCount,
		}
	}
	return roomCounts, rows.Err()
}

func (r *notificationDataStatements) SelectMaxID(ctx context.Context, txn *sql.Tx) (int64, error) {
	var id int64
	err := sqlutil.TxStmt(txn, r.selectMaxID).QueryRowContext(ctx).Scan(&id)
	return id, err
}
