// Copyright 2017 Vector Creations Ltd
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
	"strings"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/sqlite3/deltas"
	"github.com/matrix-org/dendrite/userapi/storage/tables"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const devicesSchema = `
-- This sequence is used for automatic allocation of session_id.
-- CREATE SEQUENCE IF NOT EXISTS device_session_id_seq START 1;

-- Stores data about devices.
CREATE TABLE IF NOT EXISTS userapi_devices (
    access_token TEXT PRIMARY KEY,
    session_id INTEGER,
    device_id TEXT ,
    localpart TEXT ,
	server_name TEXT NOT NULL,
    created_ts BIGINT,
    display_name TEXT,
    last_seen_ts BIGINT,
    ip TEXT,
    user_agent TEXT,

	UNIQUE (localpart, server_name, device_id)
);
`

const insertDeviceSQL = "" +
	"INSERT INTO userapi_devices (device_id, localpart, server_name, access_token, created_ts, display_name, session_id, last_seen_ts, ip, user_agent)" +
	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)"

const selectDevicesCountSQL = "" +
	"SELECT COUNT(access_token) FROM userapi_devices"

const selectDeviceByTokenSQL = "" +
	"SELECT session_id, device_id, localpart, server_name FROM userapi_devices WHERE access_token = $1"

const selectDeviceByIDSQL = "" +
	"SELECT display_name, last_seen_ts, ip FROM userapi_devices WHERE localpart = $1 AND server_name = $2 AND device_id = $3"

const selectDevicesByLocalpartSQL = "" +
	"SELECT device_id, display_name, last_seen_ts, ip, user_agent FROM userapi_devices WHERE localpart = $1 AND server_name = $2 AND device_id != $3 ORDER BY last_seen_ts DESC"

const updateDeviceNameSQL = "" +
	"UPDATE userapi_devices SET display_name = $1 WHERE localpart = $2 AND server_name = $3 AND device_id = $4"

const deleteDeviceSQL = "" +
	"DELETE FROM userapi_devices WHERE device_id = $1 AND localpart = $2 AND server_name = $3"

const deleteDevicesByLocalpartSQL = "" +
	"DELETE FROM userapi_devices WHERE localpart = $1 AND server_name = $2 AND device_id != $3"

const deleteDevicesSQL = "" +
	"DELETE FROM userapi_devices WHERE localpart = $1 AND server_name = $2 AND device_id IN ($3)"

const selectDevicesByIDSQL = "" +
	"SELECT device_id, localpart, server_name, display_name, last_seen_ts FROM userapi_devices WHERE device_id IN ($1) ORDER BY last_seen_ts DESC"

const updateDeviceLastSeen = "" +
	"UPDATE userapi_devices SET last_seen_ts = $1, ip = $2, user_agent = $3 WHERE localpart = $4 AND server_name = $5 AND device_id = $6"

type devicesStatements struct {
	db                           *sql.DB
	insertDeviceStmt             *sql.Stmt
	selectDevicesCountStmt       *sql.Stmt
	selectDeviceByTokenStmt      *sql.Stmt
	selectDeviceByIDStmt         *sql.Stmt
	selectDevicesByIDStmt        *sql.Stmt
	selectDevicesByLocalpartStmt *sql.Stmt
	updateDeviceNameStmt         *sql.Stmt
	updateDeviceLastSeenStmt     *sql.Stmt
	deleteDeviceStmt             *sql.Stmt
	deleteDevicesByLocalpartStmt *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
}

func NewSQLiteDevicesTable(db *sql.DB, serverName gomatrixserverlib.ServerName) (tables.DevicesTable, error) {
	s := &devicesStatements{
		db:         db,
		serverName: serverName,
	}
	_, err := db.Exec(devicesSchema)
	if err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrator(db)
	m.AddMigrations(sqlutil.Migration{
		Version: "userapi: add last_seen_ts",
		Up:      deltas.UpLastSeenTSIP,
	})
	if err = m.Up(context.Background()); err != nil {
		return nil, err
	}

	return s, sqlutil.StatementList{
		{&s.insertDeviceStmt, insertDeviceSQL},
		{&s.selectDevicesCountStmt, selectDevicesCountSQL},
		{&s.selectDeviceByTokenStmt, selectDeviceByTokenSQL},
		{&s.selectDeviceByIDStmt, selectDeviceByIDSQL},
		{&s.selectDevicesByLocalpartStmt, selectDevicesByLocalpartSQL},
		{&s.updateDeviceNameStmt, updateDeviceNameSQL},
		{&s.deleteDeviceStmt, deleteDeviceSQL},
		{&s.deleteDevicesByLocalpartStmt, deleteDevicesByLocalpartSQL},
		{&s.selectDevicesByIDStmt, selectDevicesByIDSQL},
		{&s.updateDeviceLastSeenStmt, updateDeviceLastSeen},
	}.Prepare(db)
}

// insertDevice creates a new device. Returns an error if any device with the same access token already exists.
// Returns an error if the user already has a device with the given device ID.
// Returns the device on success.
func (s *devicesStatements) InsertDevice(
	ctx context.Context, txn *sql.Tx, id string,
	localpart string, serverName gomatrixserverlib.ServerName,
	accessToken string, displayName *string, ipAddr, userAgent string,
) (*api.Device, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	var sessionID int64
	countStmt := sqlutil.TxStmt(txn, s.selectDevicesCountStmt)
	insertStmt := sqlutil.TxStmt(txn, s.insertDeviceStmt)
	if err := countStmt.QueryRowContext(ctx).Scan(&sessionID); err != nil {
		return nil, err
	}
	sessionID++
	if _, err := insertStmt.ExecContext(ctx, id, localpart, serverName, accessToken, createdTimeMS, displayName, sessionID, createdTimeMS, ipAddr, userAgent); err != nil {
		return nil, err
	}
	return &api.Device{
		ID:          id,
		UserID:      userutil.MakeUserID(localpart, serverName),
		AccessToken: accessToken,
		SessionID:   sessionID,
		LastSeenTS:  createdTimeMS,
		LastSeenIP:  ipAddr,
		UserAgent:   userAgent,
	}, nil
}

func (s *devicesStatements) DeleteDevice(
	ctx context.Context, txn *sql.Tx, id string,
	localpart string, serverName gomatrixserverlib.ServerName,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteDeviceStmt)
	_, err := stmt.ExecContext(ctx, id, localpart, serverName)
	return err
}

func (s *devicesStatements) DeleteDevices(
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
	devices []string,
) error {
	orig := strings.Replace(deleteDevicesSQL, "($3)", sqlutil.QueryVariadicOffset(len(devices), 2), 1)
	prep, err := s.db.Prepare(orig)
	if err != nil {
		return err
	}
	stmt := sqlutil.TxStmt(txn, prep)
	params := make([]interface{}, len(devices)+2)
	params[0] = localpart
	params[1] = serverName
	for i, v := range devices {
		params[i+2] = v
	}
	_, err = stmt.ExecContext(ctx, params...)
	return err
}

func (s *devicesStatements) DeleteDevicesByLocalpart(
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
	exceptDeviceID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteDevicesByLocalpartStmt)
	_, err := stmt.ExecContext(ctx, localpart, serverName, exceptDeviceID)
	return err
}

func (s *devicesStatements) UpdateDeviceName(
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
	deviceID string, displayName *string,
) error {
	stmt := sqlutil.TxStmt(txn, s.updateDeviceNameStmt)
	_, err := stmt.ExecContext(ctx, displayName, localpart, serverName, deviceID)
	return err
}

func (s *devicesStatements) SelectDeviceByToken(
	ctx context.Context, accessToken string,
) (*api.Device, error) {
	var dev api.Device
	var localpart string
	var serverName gomatrixserverlib.ServerName
	stmt := s.selectDeviceByTokenStmt
	err := stmt.QueryRowContext(ctx, accessToken).Scan(&dev.SessionID, &dev.ID, &localpart, &serverName)
	if err == nil {
		dev.UserID = userutil.MakeUserID(localpart, serverName)
		dev.AccessToken = accessToken
	}
	return &dev, err
}

// selectDeviceByID retrieves a device from the database with the given user
// localpart and deviceID
func (s *devicesStatements) SelectDeviceByID(
	ctx context.Context,
	localpart string, serverName gomatrixserverlib.ServerName,
	deviceID string,
) (*api.Device, error) {
	var dev api.Device
	var displayName, ip sql.NullString
	stmt := s.selectDeviceByIDStmt
	var lastseenTS sql.NullInt64
	err := stmt.QueryRowContext(ctx, localpart, serverName, deviceID).Scan(&displayName, &lastseenTS, &ip)
	if err == nil {
		dev.ID = deviceID
		dev.UserID = userutil.MakeUserID(localpart, serverName)
		if displayName.Valid {
			dev.DisplayName = displayName.String
		}
		if lastseenTS.Valid {
			dev.LastSeenTS = lastseenTS.Int64
		}
		if ip.Valid {
			dev.LastSeenIP = ip.String
		}
	}
	return &dev, err
}

func (s *devicesStatements) SelectDevicesByLocalpart(
	ctx context.Context, txn *sql.Tx,
	localpart string, serverName gomatrixserverlib.ServerName,
	exceptDeviceID string,
) ([]api.Device, error) {
	devices := []api.Device{}
	rows, err := sqlutil.TxStmt(txn, s.selectDevicesByLocalpartStmt).QueryContext(ctx, localpart, serverName, exceptDeviceID)

	if err != nil {
		return devices, err
	}

	var dev api.Device
	var lastseents sql.NullInt64
	var id, displayname, ip, useragent sql.NullString
	for rows.Next() {
		err = rows.Scan(&id, &displayname, &lastseents, &ip, &useragent)
		if err != nil {
			return devices, err
		}
		if id.Valid {
			dev.ID = id.String
		}
		if displayname.Valid {
			dev.DisplayName = displayname.String
		}
		if lastseents.Valid {
			dev.LastSeenTS = lastseents.Int64
		}
		if ip.Valid {
			dev.LastSeenIP = ip.String
		}
		if useragent.Valid {
			dev.UserAgent = useragent.String
		}

		dev.UserID = userutil.MakeUserID(localpart, serverName)
		devices = append(devices, dev)
	}

	return devices, nil
}

func (s *devicesStatements) SelectDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error) {
	sqlQuery := strings.Replace(selectDevicesByIDSQL, "($1)", sqlutil.QueryVariadic(len(deviceIDs)), 1)
	iDeviceIDs := make([]interface{}, len(deviceIDs))
	for i := range deviceIDs {
		iDeviceIDs[i] = deviceIDs[i]
	}

	rows, err := s.db.QueryContext(ctx, sqlQuery, iDeviceIDs...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectDevicesByID: rows.close() failed")
	var devices []api.Device
	var dev api.Device
	var localpart string
	var serverName gomatrixserverlib.ServerName
	var displayName sql.NullString
	var lastseents sql.NullInt64
	for rows.Next() {
		if err := rows.Scan(&dev.ID, &localpart, &serverName, &displayName, &lastseents); err != nil {
			return nil, err
		}
		if displayName.Valid {
			dev.DisplayName = displayName.String
		}
		if lastseents.Valid {
			dev.LastSeenTS = lastseents.Int64
		}
		dev.UserID = userutil.MakeUserID(localpart, serverName)
		devices = append(devices, dev)
	}
	return devices, rows.Err()
}

func (s *devicesStatements) UpdateDeviceLastSeen(ctx context.Context, txn *sql.Tx, localpart string, serverName gomatrixserverlib.ServerName, deviceID, ipAddr, userAgent string) error {
	lastSeenTs := time.Now().UnixNano() / 1000000
	stmt := sqlutil.TxStmt(txn, s.updateDeviceLastSeenStmt)
	_, err := stmt.ExecContext(ctx, lastSeenTs, ipAddr, userAgent, localpart, serverName, deviceID)
	return err
}
