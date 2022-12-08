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
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

type notificationsStatements struct {
	insertStmt             *sql.Stmt
	deleteUpToStmt         *sql.Stmt
	updateReadStmt         *sql.Stmt
	selectStmt             *sql.Stmt
	selectCountStmt        *sql.Stmt
	selectRoomCountsStmt   *sql.Stmt
	cleanNotificationsStmt *sql.Stmt
}

const notificationSchema = `
CREATE TABLE IF NOT EXISTS userapi_notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
	localpart TEXT NOT NULL,
	server_name TEXT NOT NULL,
	room_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
	stream_pos BIGINT NOT NULL,
    ts_ms BIGINT NOT NULL,
    highlight BOOLEAN NOT NULL,
    notification_json TEXT NOT NULL,
    read BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS userapi_notification_localpart_room_id_event_id_idx ON userapi_notifications(localpart, server_name, room_id, event_id);
CREATE INDEX IF NOT EXISTS userapi_notification_localpart_room_id_id_idx ON userapi_notifications(localpart, server_name, room_id, id);
CREATE INDEX IF NOT EXISTS userapi_notification_localpart_id_idx ON userapi_notifications(localpart, server_name, id);
`

const insertNotificationSQL = "" +
	"INSERT INTO userapi_notifications (localpart, server_name, room_id, event_id, stream_pos, ts_ms, highlight, notification_json) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"

const deleteNotificationsUpToSQL = "" +
	"DELETE FROM userapi_notifications WHERE localpart = $1 AND server_name = $2 AND room_id = $3 AND stream_pos <= $4"

const updateNotificationReadSQL = "" +
	"UPDATE userapi_notifications SET read = $1 WHERE localpart = $2 AND server_name = $3 AND room_id = $4 AND stream_pos <= $5 AND read <> $1"

const selectNotificationSQL = "" +
	"SELECT id, room_id, ts_ms, read, notification_json FROM userapi_notifications WHERE localpart = $1 AND server_name = $2 AND id > $3 AND (" +
	"(($4 & 1) <> 0 AND highlight) OR (($4 & 2) <> 0 AND NOT highlight)" +
	") AND NOT read ORDER BY localpart, id LIMIT $5"

const selectNotificationCountSQL = "" +
	"SELECT COUNT(*) FROM userapi_notifications WHERE localpart = $1 AND server_name = $2 AND (" +
	"(($3 & 1) <> 0 AND highlight) OR (($3 & 2) <> 0 AND NOT highlight)" +
	") AND NOT read"

const selectRoomNotificationCountsSQL = "" +
	"SELECT COUNT(*), COUNT(*) FILTER (WHERE highlight) FROM userapi_notifications " +
	"WHERE localpart = $1 AND server_name = $2 AND room_id = $3 AND NOT read"

const cleanNotificationsSQL = "" +
	"DELETE FROM userapi_notifications WHERE" +
	" (highlight = FALSE AND ts_ms < $1) OR (highlight = TRUE AND ts_ms < $2)"

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
		{&s.cleanNotificationsStmt, cleanNotificationsSQL},
	}.Prepare(db)
}

func (s *notificationsStatements) Clean(ctx context.Context, txn *sql.Tx) error {
	_, err := sqlutil.TxStmt(txn, s.cleanNotificationsStmt).ExecContext(
		ctx,
		time.Now().AddDate(0, 0, -1).UnixNano()/int64(time.Millisecond), // keep non-highlights for a day
		time.Now().AddDate(0, -1, 0).UnixNano()/int64(time.Millisecond), // keep highlights for a month
	)
	return err
}

// Insert inserts a notification into the database.
func (s *notificationsStatements) Insert(ctx context.Context, txn *sql.Tx, localpart string, serverName gomatrixserverlib.ServerName, eventID string, pos uint64, highlight bool, n *api.Notification) error {
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
	_, err = sqlutil.TxStmt(txn, s.insertStmt).ExecContext(ctx, localpart, serverName, roomID, eventID, pos, tsMS, highlight, string(bs))
	return err
}

// DeleteUpTo deletes all previous notifications, up to and including the event.
func (s *notificationsStatements) DeleteUpTo(ctx context.Context, txn *sql.Tx, localpart string, serverName gomatrixserverlib.ServerName, roomID string, pos uint64) (affected bool, _ error) {
	res, err := sqlutil.TxStmt(txn, s.deleteUpToStmt).ExecContext(ctx, localpart, serverName, roomID, pos)
	if err != nil {
		return false, err
	}
	nrows, err := res.RowsAffected()
	if err != nil {
		return true, err
	}
	log.WithFields(log.Fields{"localpart": localpart, "room_id": roomID, "stream_pos": pos}).Tracef("DeleteUpTo: %d rows affected", nrows)
	return nrows > 0, nil
}

// UpdateRead updates the "read" value for an event.
func (s *notificationsStatements) UpdateRead(ctx context.Context, txn *sql.Tx, localpart string, serverName gomatrixserverlib.ServerName, roomID string, pos uint64, v bool) (affected bool, _ error) {
	res, err := sqlutil.TxStmt(txn, s.updateReadStmt).ExecContext(ctx, v, localpart, serverName, roomID, pos)
	if err != nil {
		return false, err
	}
	nrows, err := res.RowsAffected()
	if err != nil {
		return true, err
	}
	log.WithFields(log.Fields{"localpart": localpart, "room_id": roomID, "stream_pos": pos}).Tracef("UpdateRead: %d rows affected", nrows)
	return nrows > 0, nil
}

func (s *notificationsStatements) Select(ctx context.Context, txn *sql.Tx, localpart string, serverName gomatrixserverlib.ServerName, fromID int64, limit int, filter tables.NotificationFilter) ([]*api.Notification, int64, error) {
	rows, err := sqlutil.TxStmt(txn, s.selectStmt).QueryContext(ctx, localpart, serverName, fromID, uint32(filter), limit)

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

func (s *notificationsStatements) SelectCount(ctx context.Context, txn *sql.Tx, localpart string, serverName gomatrixserverlib.ServerName, filter tables.NotificationFilter) (count int64, err error) {
	err = sqlutil.TxStmt(txn, s.selectCountStmt).QueryRowContext(ctx, localpart, serverName, uint32(filter)).Scan(&count)
	return
}

func (s *notificationsStatements) SelectRoomCounts(ctx context.Context, txn *sql.Tx, localpart string, serverName gomatrixserverlib.ServerName, roomID string) (total int64, highlight int64, err error) {
	err = sqlutil.TxStmt(txn, s.selectRoomCountsStmt).QueryRowContext(ctx, localpart, serverName, roomID).Scan(&total, &highlight)
	return
}
