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

	"github.com/lib/pq"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
)

func NewPostgresNotificationDataTable(db *sql.DB) (tables.NotificationData, error) {
	_, err := db.Exec(notificationDataSchema)
	if err != nil {
		return nil, err
	}
	r := &notificationDataStatements{}
	return r, sqlutil.StatementList{
		{&r.upsertRoomUnreadCounts, upsertRoomUnreadNotificationCountsSQL},
		{&r.selectUserUnreadCountsForRooms, selectUserUnreadNotificationsForRooms},
		{&r.selectMaxID, selectMaxNotificationIDSQL},
	}.Prepare(db)
}

type notificationDataStatements struct {
	upsertRoomUnreadCounts         *sql.Stmt
	selectUserUnreadCountsForRooms *sql.Stmt
	selectMaxID                    *sql.Stmt
}

const notificationDataSchema = `
CREATE TABLE IF NOT EXISTS syncapi_notification_data (
	id BIGSERIAL PRIMARY KEY,
	user_id TEXT NOT NULL,
	room_id TEXT NOT NULL,
	notification_count BIGINT NOT NULL DEFAULT 0,
	highlight_count BIGINT NOT NULL DEFAULT 0,
	CONSTRAINT syncapi_notification_data_unique UNIQUE (user_id, room_id)
);`

const upsertRoomUnreadNotificationCountsSQL = `INSERT INTO syncapi_notification_data
  (user_id, room_id, notification_count, highlight_count)
  VALUES ($1, $2, $3, $4)
  ON CONFLICT (user_id, room_id)
  DO UPDATE SET id = nextval('syncapi_notification_data_id_seq'), notification_count = $3, highlight_count = $4
  RETURNING id`

const selectUserUnreadNotificationsForRooms = `SELECT room_id, notification_count, highlight_count
	FROM syncapi_notification_data
	WHERE user_id = $1 AND
	      room_id = ANY($2)`

const selectMaxNotificationIDSQL = `SELECT CASE COUNT(*) WHEN 0 THEN 0 ELSE MAX(id) END FROM syncapi_notification_data`

func (r *notificationDataStatements) UpsertRoomUnreadCounts(ctx context.Context, txn *sql.Tx, userID, roomID string, notificationCount, highlightCount int) (pos types.StreamPosition, err error) {
	err = sqlutil.TxStmt(txn, r.upsertRoomUnreadCounts).QueryRowContext(ctx, userID, roomID, notificationCount, highlightCount).Scan(&pos)
	return
}

func (r *notificationDataStatements) SelectUserUnreadCountsForRooms(
	ctx context.Context, txn *sql.Tx, userID string, roomIDs []string,
) (map[string]*eventutil.NotificationData, error) {
	rows, err := sqlutil.TxStmt(txn, r.selectUserUnreadCountsForRooms).QueryContext(ctx, userID, pq.Array(roomIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectUserUnreadCountsForRooms: rows.close() failed")

	roomCounts := map[string]*eventutil.NotificationData{}
	var roomID string
	var notificationCount, highlightCount int
	for rows.Next() {
		if err = rows.Scan(&roomID, &notificationCount, &highlightCount); err != nil {
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
