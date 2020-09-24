// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	"encoding/json"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
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
	content TEXT NOT NULL,
	-- The token that was supplied to the /sync at the time that this
	-- message was included in a sync response, or NULL if we haven't
	-- included it in a /sync response yet.
	sent_by_token TEXT
);
`

const insertSendToDeviceMessageSQL = `
	INSERT INTO syncapi_send_to_device (user_id, device_id, content)
	  VALUES ($1, $2, $3)
`

const countSendToDeviceMessagesSQL = `
	SELECT COUNT(*)
	  FROM syncapi_send_to_device
	  WHERE user_id = $1 AND device_id = $2
`

const selectSendToDeviceMessagesSQL = `
	SELECT id, user_id, device_id, content, sent_by_token
	  FROM syncapi_send_to_device
	  WHERE user_id = $1 AND device_id = $2
	  ORDER BY id DESC
`

const updateSentSendToDeviceMessagesSQL = `
	UPDATE syncapi_send_to_device SET sent_by_token = $1
	  WHERE id = ANY($2)
`

const deleteSendToDeviceMessagesSQL = `
	DELETE FROM syncapi_send_to_device WHERE id = ANY($1)
`

type sendToDeviceStatements struct {
	insertSendToDeviceMessageStmt      *sql.Stmt
	countSendToDeviceMessagesStmt      *sql.Stmt
	selectSendToDeviceMessagesStmt     *sql.Stmt
	updateSentSendToDeviceMessagesStmt *sql.Stmt
	deleteSendToDeviceMessagesStmt     *sql.Stmt
}

func NewPostgresSendToDeviceTable(db *sql.DB) (tables.SendToDevice, error) {
	s := &sendToDeviceStatements{}
	_, err := db.Exec(sendToDeviceSchema)
	if err != nil {
		return nil, err
	}
	if s.insertSendToDeviceMessageStmt, err = db.Prepare(insertSendToDeviceMessageSQL); err != nil {
		return nil, err
	}
	if s.countSendToDeviceMessagesStmt, err = db.Prepare(countSendToDeviceMessagesSQL); err != nil {
		return nil, err
	}
	if s.selectSendToDeviceMessagesStmt, err = db.Prepare(selectSendToDeviceMessagesSQL); err != nil {
		return nil, err
	}
	if s.updateSentSendToDeviceMessagesStmt, err = db.Prepare(updateSentSendToDeviceMessagesSQL); err != nil {
		return nil, err
	}
	if s.deleteSendToDeviceMessagesStmt, err = db.Prepare(deleteSendToDeviceMessagesSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *sendToDeviceStatements) InsertSendToDeviceMessage(
	ctx context.Context, txn *sql.Tx, userID, deviceID, content string,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.insertSendToDeviceMessageStmt).ExecContext(ctx, userID, deviceID, content)
	return
}

func (s *sendToDeviceStatements) CountSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, userID, deviceID string,
) (count int, err error) {
	row := sqlutil.TxStmt(txn, s.countSendToDeviceMessagesStmt).QueryRowContext(ctx, userID, deviceID)
	if err = row.Scan(&count); err != nil {
		return
	}
	return count, nil
}

func (s *sendToDeviceStatements) SelectSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, userID, deviceID string,
) (events []types.SendToDeviceEvent, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectSendToDeviceMessagesStmt).QueryContext(ctx, userID, deviceID)
	if err != nil {
		return
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectSendToDeviceMessages: rows.close() failed")

	for rows.Next() {
		var id types.SendToDeviceNID
		var userID, deviceID, content string
		var sentByToken *string
		if err = rows.Scan(&id, &userID, &deviceID, &content, &sentByToken); err != nil {
			return
		}
		event := types.SendToDeviceEvent{
			ID:       id,
			UserID:   userID,
			DeviceID: deviceID,
		}
		if err = json.Unmarshal([]byte(content), &event.SendToDeviceEvent); err != nil {
			return
		}
		if sentByToken != nil {
			if token, err := types.NewStreamTokenFromString(*sentByToken); err == nil {
				event.SentByToken = &token
			}
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (s *sendToDeviceStatements) UpdateSentSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, token string, nids []types.SendToDeviceNID,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.updateSentSendToDeviceMessagesStmt).ExecContext(ctx, token, pq.Array(nids))
	return
}

func (s *sendToDeviceStatements) DeleteSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, nids []types.SendToDeviceNID,
) (err error) {
	_, err = sqlutil.TxStmt(txn, s.deleteSendToDeviceMessagesStmt).ExecContext(ctx, pq.Array(nids))
	return
}
