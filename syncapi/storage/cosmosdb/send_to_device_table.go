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

package cosmosdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/sirupsen/logrus"
)

// const sendToDeviceSchema = `
// -- Stores send-to-device messages.
// CREATE TABLE IF NOT EXISTS syncapi_send_to_device (
// 	-- The ID that uniquely identifies this message.
// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
// 	-- The user ID to send the message to.
// 	user_id TEXT NOT NULL,
// 	-- The device ID to send the message to.
// 	device_id TEXT NOT NULL,
// 	-- The event content JSON.
// 	content TEXT NOT NULL
// );
// `

type SendToDeviceCosmos struct {
	ID       int64  `json:"id"`
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	Content  string `json:"content"`
}

type SendToDeviceCosmosMaxNumber struct {
	Max int64 `json:"number"`
}

type SendToDeviceCosmosData struct {
	cosmosdbapi.CosmosDocument
	SendToDevice SendToDeviceCosmos `json:"mx_syncapi_send_to_device"`
}

// const insertSendToDeviceMessageSQL = `
// 	INSERT INTO syncapi_send_to_device (user_id, device_id, content)
// 	  VALUES ($1, $2, $3)
// `

// SELECT id, user_id, device_id, content
// FROM syncapi_send_to_device
// WHERE user_id = $1 AND device_id = $2 AND id > $3 AND id <= $4
// ORDER BY id DESC
const selectSendToDeviceMessagesSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_send_to_device.user_id = @x2 " +
	"and c.mx_syncapi_send_to_device.device_id = @x3 " +
	"and c.mx_syncapi_send_to_device.id > @x4 " +
	"and c.mx_syncapi_send_to_device.id <= @x5 " +
	"order by c.mx_syncapi_send_to_device.id desc "

// DELETE FROM syncapi_send_to_device
// 	WHERE user_id = $1 AND device_id = $2 AND id < $3
const deleteSendToDeviceMessagesSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_send_to_device.user_id = @x2 " +
	"and c.mx_syncapi_send_to_device.device_id = @x3 " +
	"and c.mx_syncapi_send_to_device.id < @x4 "

// "SELECT MAX(id) FROM syncapi_send_to_device"
const selectMaxSendToDeviceIDSQL = "" +
	"select max(c.mx_syncapi_send_to_device.id) as number from c where c._cn = @x1 " +
	"and c._sid = @x2 "

type sendToDeviceStatements struct {
	db *SyncServerDatasource
	// insertSendToDeviceMessageStmt  *sql.Stmt
	selectSendToDeviceMessagesStmt string
	deleteSendToDeviceMessagesStmt string
	selectMaxSendToDeviceIDStmt    string
	tableName                      string
}

func (s *sendToDeviceStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *sendToDeviceStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func deleteSendToDevice(s *sendToDeviceStatements, ctx context.Context, dbData SendToDeviceCosmosData) error {
	var options = cosmosdbapi.GetDeleteDocumentOptions(dbData.Pk)
	var _, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Id,
		options)

	if err != nil {
		return err
	}
	return err
}

func NewCosmosDBSendToDeviceTable(db *SyncServerDatasource) (tables.SendToDevice, error) {
	s := &sendToDeviceStatements{
		db: db,
	}
	// if s.insertSendToDeviceMessageStmt, err = db.Prepare(insertSendToDeviceMessageSQL); err != nil {
	// 	return nil, err
	// }
	s.selectSendToDeviceMessagesStmt = selectSendToDeviceMessagesSQL
	s.deleteSendToDeviceMessagesStmt = deleteSendToDeviceMessagesSQL
	s.selectMaxSendToDeviceIDStmt = selectMaxSendToDeviceIDSQL
	s.tableName = "send_to_device"
	return s, nil
}

func (s *sendToDeviceStatements) InsertSendToDeviceMessage(
	ctx context.Context, txn *sql.Tx, userID, deviceID, content string,
) (pos types.StreamPosition, err error) {

	// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
	id, err := GetNextSendToDeviceID(s, ctx)
	if err != nil {
		return 0, err
	}

	pos = types.StreamPosition(id)

	// INSERT INTO syncapi_send_to_device (user_id, device_id, content)
	//   VALUES ($1, $2, $3)

	// 	NO CONSTRAINT
	docId := fmt.Sprintf("%d", pos)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := SendToDeviceCosmos{
		ID:       int64(pos),
		UserID:   userID,
		DeviceID: deviceID,
		Content:  content,
	}

	var dbData = SendToDeviceCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		SendToDevice:   data,
	}

	var optionsCreate = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		optionsCreate)

	return
}

func (s *sendToDeviceStatements) SelectSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, userID, deviceID string, from, to types.StreamPosition,
) (lastPos types.StreamPosition, events []types.SendToDeviceEvent, err error) {
	// SELECT id, user_id, device_id, content
	// FROM syncapi_send_to_device
	// WHERE user_id = $1 AND device_id = $2 AND id > $3 AND id <= $4
	// ORDER BY id DESC

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": deviceID,
		"@x4": from,
		"@x5": to,
	}

	var rows []SendToDeviceCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectSendToDeviceMessagesStmt, params, &rows)

	if err != nil {
		return
	}

	for _, item := range rows {
		var id types.StreamPosition
		var userID, deviceID, content string
		// if err = rows.Scan(&id, &userID, &deviceID, &content); err != nil {
		// 	logrus.WithError(err).Errorf("Failed to retrieve send-to-device message")
		// 	return
		// }
		id = types.StreamPosition(item.SendToDevice.ID)
		userID = item.SendToDevice.UserID
		deviceID = item.SendToDevice.DeviceID
		content = item.SendToDevice.Content
		if id > lastPos {
			lastPos = id
		}
		event := types.SendToDeviceEvent{
			ID:       id,
			UserID:   userID,
			DeviceID: deviceID,
		}
		if jsonErr := json.Unmarshal([]byte(content), &event.SendToDeviceEvent); err != nil {
			logrus.WithError(jsonErr).Errorf("Failed to unmarshal send-to-device message")
			continue
		}
		events = append(events, event)
	}
	if lastPos == 0 {
		lastPos = to
	}
	return lastPos, events, err
}

func (s *sendToDeviceStatements) DeleteSendToDeviceMessages(
	ctx context.Context, txn *sql.Tx, userID, deviceID string, pos types.StreamPosition,
) (err error) {
	// DELETE FROM syncapi_send_to_device
	// 	WHERE user_id = $1 AND device_id = $2 AND id < $3

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": deviceID,
		"@x4": pos,
	}

	// _, err = sqlutil.TxStmt(txn, s.deleteSendToDeviceMessagesStmt).ExecContext(ctx, userID, deviceID, pos)
	var rows []SendToDeviceCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.deleteSendToDeviceMessagesStmt, params, &rows)

	if err != nil {
		return err
	}

	for _, item := range rows {
		err = deleteSendToDevice(s, ctx, item)
		if err != nil {
			return err
		}
	}
	return
}

func (s *sendToDeviceStatements) SelectMaxSendToDeviceMessageID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64
	// "SELECT MAX(id) FROM syncapi_send_to_device"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": s.db.cosmosConfig.TenantName,
	}
	var rows []SendToDeviceCosmosMaxNumber
	err = cosmosdbapi.PerformQueryAllPartitions(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.selectMaxSendToDeviceIDStmt, params, &rows)

	// stmt := sqlutil.TxStmt(txn, s.selectMaxSendToDeviceIDStmt)
	// err = stmt.QueryRowContext(ctx).Scan(&nullableID)

	if rows != nil {
		nullableID.Int64 = rows[0].Max
	}

	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
