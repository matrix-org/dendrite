// Copyright 2021 Dan Peleg <dan@globekeeper.com>
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
	"encoding/json"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type notificationsStatements struct {
	insertStmt           *sql.Stmt
	deleteUpToStmt       *sql.Stmt
	updateReadStmt       *sql.Stmt
	selectStmt           *sql.Stmt
	selectCountStmt      *sql.Stmt
	selectRoomCountsStmt *sql.Stmt
}

const notificationSchema = `
CREATE TABLE IF NOT EXISTS userapi_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
	localpart TEXT NOT NULL,
	room_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
    ts_ms BIGINT NOT NULL,
    highlight BOOLEAN NOT NULL,
    notification_json TEXT NOT NULL,
    read BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS userapi_notification_localpart_room_id_event_id_idx ON userapi_notifications(localpart, room_id, event_id);
CREATE INDEX IF NOT EXISTS userapi_notification_localpart_room_id_id_idx ON userapi_notifications(localpart, room_id, id);
CREATE INDEX IF NOT EXISTS userapi_notification_localpart_id_idx ON userapi_notifications(localpart, id);
`

const insertNotificationSQL = "" +
	"INSERT INTO userapi_notifications (localpart, room_id, event_id, ts_ms, highlight, notification_json) VALUES ($1, $2, $3, $4, $5, $6)"

const deleteNotificationsUpToSQL = "" +
	"DELETE FROM userapi_notifications WHERE localpart = $1 AND room_id = $2 AND id <= (" +
	"SELECT MAX(id) FROM userapi_notifications WHERE localpart = $1 AND room_id = $2 AND event_id = $3" +
	")"

const updateNotificationReadSQL = "" +
	"UPDATE userapi_notifications SET read = $1 WHERE localpart = $2 AND room_id = $3 AND id <= (" +
	"SELECT MAX(id) FROM userapi_notifications WHERE localpart = $2 AND room_id = $3 AND event_id = $4" +
	") AND read <> $1"

const selectNotificationSQL = "" +
	"SELECT id, room_id, ts_ms, read, notification_json FROM userapi_notifications WHERE localpart = $1 AND id > $2 AND (" +
	"(($3 & 1) <> 0 AND highlight) OR (($3 & 2) <> 0 AND NOT highlight)" +
	") AND NOT read ORDER BY localpart, id LIMIT $4"

const selectNotificationCountSQL = "" +
	"SELECT COUNT(*) FROM userapi_notifications WHERE localpart = $1 AND (" +
	"(($2 & 1) <> 0 AND highlight) OR (($2 & 2) <> 0 AND NOT highlight)" +
	") AND NOT read"

const selectRoomNotificationCountsSQL = "" +
	"SELECT COUNT(*), COUNT(*) FILTER (WHERE highlight) FROM userapi_notifications " +
	"WHERE localpart = $1 AND room_id = $2 AND NOT read"

func NewSQLiteNotificationTable(db *sql.DB) (tables.NotificationTable, error) {
	s := &notificationsStatements{}
	_, err := db.Exec(notificationSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertStmt, insertNotificationSQL},
		{&s.deleteUpToStmt, deleteNotificationsUpToSQL},
		{&s.updateReadStmt, updateNotificationReadSQL},
		{&s.selectStmt, selectNotificationSQL},
		{&s.selectCountStmt, selectNotificationCountSQL},
		{&s.selectRoomCountsStmt, selectRoomNotificationCountsSQL},
	}.Prepare(db)
}

// Insert inserts a notification into the database.
func (s *notificationsStatements) Insert(ctx context.Context, txn *sql.Tx, localpart, eventID string, highlight bool, n *api.Notification) error {
	roomID, tsMS := n.RoomID, n.TS
	nn := *n
	// Clears out fields that have their own columns to (1) shrink the
	// data and (2) avoid difficult-to-debug inconsistency bugs.
	nn.RoomID = ""
	nn.TS, nn.Read = 0, false
	bs, err := json.Marshal(nn)
	if err != nil {
		return err
	}
	_, err = sqlutil.TxStmt(txn, s.insertStmt).ExecContext(ctx, localpart, roomID, eventID, tsMS, highlight, string(bs))
	return err
}

// DeleteUpTo deletes all previous notifications, up to and including the event.
func (s *notificationsStatements) DeleteUpTo(ctx context.Context, txn *sql.Tx, localpart, roomID, eventID string) (affected bool, _ error) {
	res, err := sqlutil.TxStmt(txn, s.deleteUpToStmt).ExecContext(ctx, localpart, roomID, eventID)
	if err != nil {
		return false, err
	}
	nrows, err := res.RowsAffected()
	if err != nil {
		return true, err
	}
	log.WithFields(log.Fields{"localpart": localpart, "room_id": roomID, "event_id": eventID}).Tracef("DeleteUpTo: %d rows affected", nrows)
	return nrows > 0, nil
}

// UpdateRead updates the "read" value for an event.
func (s *notificationsStatements) UpdateRead(ctx context.Context, txn *sql.Tx, localpart, roomID, eventID string, v bool) (affected bool, _ error) {
	res, err := sqlutil.TxStmt(txn, s.updateReadStmt).ExecContext(ctx, v, localpart, roomID, eventID)
	if err != nil {
		return false, err
	}
	nrows, err := res.RowsAffected()
	if err != nil {
		return true, err
	}
	log.WithFields(log.Fields{"localpart": localpart, "room_id": roomID, "event_id": eventID}).Tracef("UpdateRead: %d rows affected", nrows)
	return nrows > 0, nil
}

func (s *notificationsStatements) Select(ctx context.Context, txn *sql.Tx, localpart string, fromID int64, limit int, filter tables.NotificationFilter) ([]*api.Notification, int64, error) {
	rows, err := sqlutil.TxStmt(txn, s.selectStmt).QueryContext(ctx, localpart, fromID, uint32(filter), limit)

	if err != nil {
		return nil, 0, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "notifications.Select: rows.Close() failed")

	var maxID int64 = -1
	var notifs []*api.Notification
	for rows.Next() {
		var id int64
		var roomID string
		var ts gomatrixserverlib.Timestamp
		var read bool
		var jsonStr string
		err = rows.Scan(
			&id,
			&roomID,
			&ts,
			&read,
			&jsonStr)
		if err != nil {
			return nil, 0, err
		}

		var n api.Notification
		err := json.Unmarshal([]byte(jsonStr), &n)
		if err != nil {
			return nil, 0, err
		}
		n.RoomID = roomID
		n.TS = ts
		n.Read = read
		notifs = append(notifs, &n)

		if maxID < id {
			maxID = id
		}
	}
	return notifs, maxID, rows.Err()
}

func (s *notificationsStatements) SelectCount(ctx context.Context, txn *sql.Tx, localpart string, filter tables.NotificationFilter) (int64, error) {
	rows, err := sqlutil.TxStmt(txn, s.selectCountStmt).QueryContext(ctx, localpart, uint32(filter))

	if err != nil {
		return 0, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "notifications.Select: rows.Close() failed")

	if rows.Next() {
		var count int64
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}

		return count, nil
	}
	return 0, rows.Err()
}

func (s *notificationsStatements) SelectRoomCounts(ctx context.Context, txn *sql.Tx, localpart, roomID string) (total int64, highlight int64, _ error) {
	rows, err := sqlutil.TxStmt(txn, s.selectRoomCountsStmt).QueryContext(ctx, localpart, roomID)

	if err != nil {
		return 0, 0, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "notifications.Select: rows.Close() failed")

	if rows.Next() {
		var total, highlight int64
		if err := rows.Scan(&total, &highlight); err != nil {
			return 0, 0, err
		}

		return total, highlight, nil
	}
	return 0, 0, rows.Err()
}
