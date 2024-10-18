// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/syncapi/storage/postgres/deltas"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/sirupsen/logrus"
)

const sendToDeviceSchema = `
CREATE SEQUENCE IF NOT EXISTS syncapi_send_to_device_id;

-- Stores send-to-device messages.
CREATE TABLE IF NOT EXISTS syncapi_send_to_device (
	-- The ID that uniquely identifies this message.
	id BIGINT PRIMARY KEY DEFAULT nextval('syncapi_send_to_device_id'),
	-- The user ID to send the message to.
	user_id TEXT NOT NULL,
	-- The device ID to send the message to.
	device_id TEXT NOT NULL,
	-- The event content JSON.
	content TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS syncapi_send_to_device_user_id_device_id_idx ON syncapi_send_to_device(user_id, device_id);
`

const insertSendToDeviceMessageSQL = `
	INSERT INTO syncapi_send_to_device (user_id, device_id, content)
	  VALUES ($1, $2, $3)
	  RETURNING id
`

const selectSendToDeviceMessagesSQL = `
	SELECT id, user_id, device_id, content
	  FROM syncapi_send_to_device
	  WHERE user_id = $1 AND device_id = $2 AND id > $3 AND id <= $4
	  ORDER BY id ASC
`

const deleteSendToDeviceMessagesSQL = `
	DELETE FROM syncapi_send_to_device
	  WHERE user_id = $1 AND device_id = $2 AND id <= $3
`

const selectMaxSendToDeviceIDSQL = "" +
	"SELECT MAX(id) FROM syncapi_send_to_device"

type sendToDeviceStatements struct {
	insertSendToDeviceMessageStmt  *sql.Stmt
	selectSendToDeviceMessagesStmt *sql.Stmt
	deleteSendToDeviceMessagesStmt *sql.Stmt
	selectMaxSendToDeviceIDStmt    *sql.Stmt
}

func NewPostgresSendToDeviceTable(db *sql.DB) (tables.SendToDevice, error) {
	s := &sendToDeviceStatements{}
	_, err := db.Exec(sendToDeviceSchema)
	if err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrator(db)
	m.AddMigrations(sqlutil.Migration{
		Version: "syncapi: drop sent_by_token",
		Up:      deltas.UpRemoveSendToDeviceSentColumn,
	})
	err = m.Up(context.Background())
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.insertSendToDeviceMessageStmt, insertSendToDeviceMessageSQL},
		{&s.selectSendToDeviceMessagesStmt, selectSendToDeviceMessagesSQL},
		{&s.deleteSendToDeviceMessagesStmt, deleteSendToDeviceMessagesSQL},
		{&s.selectMaxSendToDeviceIDStmt, selectMaxSendToDeviceIDSQL},
	}.Prepare(db)
}

func (s *sendToDeviceStatements) InsertSendToDeviceMessage(
	ctx context.Context, txn *sql.Tx, userID, deviceID, content string,
) (pos types.StreamPosition, err error) {
	err = sqlutil.TxStmt(txn, s.insertSendToDeviceMessageStmt).QueryRowContext(ctx, userID, deviceID, content).Scan(&pos)
	return
}

func (s *sendToDeviceStatements) SelectSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, userID, deviceID string, from, to types.StreamPosition,
) (lastPos types.StreamPosition, events []types.SendToDeviceEvent, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectSendToDeviceMessagesStmt).QueryContext(ctx, userID, deviceID, from, to)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectSendToDeviceMessages: rows.close() failed")

	for rows.Next() {
		var id types.StreamPosition
		var userID, deviceID, content string
		if err = rows.Scan(&id, &userID, &deviceID, &content); err != nil {
			return
		}
		event := types.SendToDeviceEvent{
			ID:       id,
			UserID:   userID,
			DeviceID: deviceID,
		}
		if err = json.Unmarshal([]byte(content), &event.SendToDeviceEvent); err != nil {
			logrus.WithError(err).Errorf("Failed to unmarshal send-to-device message")
			continue
		}
		if id > lastPos {
			lastPos = id
		}
		events = append(events, event)
	}
	if lastPos == 0 {
		lastPos = to
	}
	return lastPos, events, rows.Err()
}

func (s *sendToDeviceStatements) DeleteSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, userID, deviceID string, pos types.StreamPosition,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteSendToDeviceMessagesStmt).ExecContext(ctx, userID, deviceID, pos)
	return
}

func (s *sendToDeviceStatements) SelectMaxSendToDeviceMessageID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	stmt := sqlutil.TxStmt(txn, s.selectMaxSendToDeviceIDStmt)
	err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
