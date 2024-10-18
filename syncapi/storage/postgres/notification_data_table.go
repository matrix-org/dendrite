// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/eventutil"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
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
		{&r.purgeNotificationData, purgeNotificationDataSQL},
	}.Prepare(db)
}

type notificationDataStatements struct {
	upsertRoomUnreadCounts         *sql.Stmt
	selectUserUnreadCountsForRooms *sql.Stmt
	selectMaxID                    *sql.Stmt
	purgeNotificationData          *sql.Stmt
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

const purgeNotificationDataSQL = "" +
	"DELETE FROM syncapi_notification_data WHERE room_id = $1"

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

func (s *notificationDataStatements) PurgeNotificationData(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {
	_, err := sqlutil.TxStmt(txn, s.purgeNotificationData).ExecContext(ctx, roomID)
	return err
}
