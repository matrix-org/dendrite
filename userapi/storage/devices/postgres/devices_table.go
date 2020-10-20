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

package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

const devicesSchema = `
-- This sequence is used for automatic allocation of session_id.
CREATE SEQUENCE IF NOT EXISTS device_session_id_seq START 1;

-- Stores data about devices.
CREATE TABLE IF NOT EXISTS device_devices (
    -- The access token granted to this device. This has to be the primary key
    -- so we can distinguish which device is making a given request.
    access_token TEXT NOT NULL PRIMARY KEY,
    -- The auto-allocated unique ID of the session identified by the access token.
    -- This can be used as a secure substitution of the access token in situations
    -- where data is associated with access tokens (e.g. transaction storage),
    -- so we don't have to store users' access tokens everywhere.
    session_id BIGINT NOT NULL DEFAULT nextval('device_session_id_seq'),
    -- The device identifier. This only needs to uniquely identify a device for a given user, not globally.
    -- access_tokens will be clobbered based on the device ID for a user.
    device_id TEXT NOT NULL,
    -- The Matrix user ID localpart for this device. This is preferable to storing the full user_id
    -- as it is smaller, makes it clearer that we only manage devices for our own users, and may make
    -- migration to different domain names easier.
    localpart TEXT NOT NULL,
    -- When this devices was first recognised on the network, as a unix timestamp (ms resolution).
    created_ts BIGINT NOT NULL,
    -- The display name, human friendlier than device_id and updatable
    display_name TEXT,
	-- The time the device was last used, as a unix timestamp (ms resolution).
	last_seen_ts BIGINT NOT NULL,
	-- The last seen IP address of this device
	ip TEXT,
	-- User agent of this device
	user_agent TEXT
                                          
    -- TODO: device keys, device display names, token restrictions (if 3rd-party OAuth app)
);

-- Device IDs must be unique for a given user.
CREATE UNIQUE INDEX IF NOT EXISTS device_localpart_id_idx ON device_devices(localpart, device_id);
`

const insertDeviceSQL = "" +
	"INSERT INTO device_devices(device_id, localpart, access_token, created_ts, display_name, last_seen_ts, ip, user_agent) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)" +
	" RETURNING session_id"

const selectDeviceByTokenSQL = "" +
	"SELECT session_id, device_id, localpart FROM device_devices WHERE access_token = $1"

const selectDeviceByIDSQL = "" +
	"SELECT display_name FROM device_devices WHERE localpart = $1 and device_id = $2"

const selectDevicesByLocalpartSQL = "" +
	"SELECT device_id, display_name FROM device_devices WHERE localpart = $1 AND device_id != $2"

const updateDeviceNameSQL = "" +
	"UPDATE device_devices SET display_name = $1 WHERE localpart = $2 AND device_id = $3"

const deleteDeviceSQL = "" +
	"DELETE FROM device_devices WHERE device_id = $1 AND localpart = $2"

const deleteDevicesByLocalpartSQL = "" +
	"DELETE FROM device_devices WHERE localpart = $1 AND device_id != $2"

const deleteDevicesSQL = "" +
	"DELETE FROM device_devices WHERE localpart = $1 AND device_id = ANY($2)"

const selectDevicesByIDSQL = "" +
	"SELECT device_id, localpart, display_name FROM device_devices WHERE device_id = ANY($1)"

const updateDeviceLastSeen = "" +
	"UPDATE device_devices SET last_seen_ts = $1, ip = $2 WHERE device_id = $3"

type devicesStatements struct {
	insertDeviceStmt             *sql.Stmt
	selectDeviceByTokenStmt      *sql.Stmt
	selectDeviceByIDStmt         *sql.Stmt
	selectDevicesByLocalpartStmt *sql.Stmt
	selectDevicesByIDStmt        *sql.Stmt
	updateDeviceNameStmt         *sql.Stmt
	updateDeviceLastSeenStmt     *sql.Stmt
	deleteDeviceStmt             *sql.Stmt
	deleteDevicesByLocalpartStmt *sql.Stmt
	deleteDevicesStmt            *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
}

func (s *devicesStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(devicesSchema)
	return err
}

func (s *devicesStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
	if s.insertDeviceStmt, err = db.Prepare(insertDeviceSQL); err != nil {
		return
	}
	if s.selectDeviceByTokenStmt, err = db.Prepare(selectDeviceByTokenSQL); err != nil {
		return
	}
	if s.selectDeviceByIDStmt, err = db.Prepare(selectDeviceByIDSQL); err != nil {
		return
	}
	if s.selectDevicesByLocalpartStmt, err = db.Prepare(selectDevicesByLocalpartSQL); err != nil {
		return
	}
	if s.updateDeviceNameStmt, err = db.Prepare(updateDeviceNameSQL); err != nil {
		return
	}
	if s.deleteDeviceStmt, err = db.Prepare(deleteDeviceSQL); err != nil {
		return
	}
	if s.deleteDevicesByLocalpartStmt, err = db.Prepare(deleteDevicesByLocalpartSQL); err != nil {
		return
	}
	if s.deleteDevicesStmt, err = db.Prepare(deleteDevicesSQL); err != nil {
		return
	}
	if s.selectDevicesByIDStmt, err = db.Prepare(selectDevicesByIDSQL); err != nil {
		return
	}
	if s.updateDeviceLastSeenStmt, err = db.Prepare(updateDeviceLastSeen); err != nil {
		return
	}
	s.serverName = server
	return
}

// insertDevice creates a new device. Returns an error if any device with the same access token already exists.
// Returns an error if the user already has a device with the given device ID.
// Returns the device on success.
func (s *devicesStatements) insertDevice(
	ctx context.Context, txn *sql.Tx, id, localpart, accessToken string,
	displayName *string, ipAddr, userAgent string,
) (*api.Device, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	var sessionID int64
	stmt := sqlutil.TxStmt(txn, s.insertDeviceStmt)
	if err := stmt.QueryRowContext(ctx, id, localpart, accessToken, createdTimeMS, displayName, createdTimeMS, ipAddr, userAgent).Scan(&sessionID); err != nil {
		return nil, err
	}
	return &api.Device{
		ID:          id,
		UserID:      userutil.MakeUserID(localpart, s.serverName),
		AccessToken: accessToken,
		SessionID:   sessionID,
		LastSeenTS:  createdTimeMS,
		LastSeenIP:  ipAddr,
		UserAgent:   userAgent,
	}, nil
}

// deleteDevice removes a single device by id and user localpart.
func (s *devicesStatements) deleteDevice(
	ctx context.Context, txn *sql.Tx, id, localpart string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteDeviceStmt)
	_, err := stmt.ExecContext(ctx, id, localpart)
	return err
}

// deleteDevices removes a single or multiple devices by ids and user localpart.
// Returns an error if the execution failed.
func (s *devicesStatements) deleteDevices(
	ctx context.Context, txn *sql.Tx, localpart string, devices []string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteDevicesStmt)
	_, err := stmt.ExecContext(ctx, localpart, pq.Array(devices))
	return err
}

// deleteDevicesByLocalpart removes all devices for the
// given user localpart.
func (s *devicesStatements) deleteDevicesByLocalpart(
	ctx context.Context, txn *sql.Tx, localpart, exceptDeviceID string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deleteDevicesByLocalpartStmt)
	_, err := stmt.ExecContext(ctx, localpart, exceptDeviceID)
	return err
}

func (s *devicesStatements) updateDeviceName(
	ctx context.Context, txn *sql.Tx, localpart, deviceID string, displayName *string,
) error {
	stmt := sqlutil.TxStmt(txn, s.updateDeviceNameStmt)
	_, err := stmt.ExecContext(ctx, displayName, localpart, deviceID)
	return err
}

func (s *devicesStatements) selectDeviceByToken(
	ctx context.Context, accessToken string,
) (*api.Device, error) {
	var dev api.Device
	var localpart string
	stmt := s.selectDeviceByTokenStmt
	err := stmt.QueryRowContext(ctx, accessToken).Scan(&dev.SessionID, &dev.ID, &localpart)
	if err == nil {
		dev.UserID = userutil.MakeUserID(localpart, s.serverName)
		dev.AccessToken = accessToken
	}
	return &dev, err
}

// selectDeviceByID retrieves a device from the database with the given user
// localpart and deviceID
func (s *devicesStatements) selectDeviceByID(
	ctx context.Context, localpart, deviceID string,
) (*api.Device, error) {
	var dev api.Device
	var displayName sql.NullString
	stmt := s.selectDeviceByIDStmt
	err := stmt.QueryRowContext(ctx, localpart, deviceID).Scan(&displayName)
	if err == nil {
		dev.ID = deviceID
		dev.UserID = userutil.MakeUserID(localpart, s.serverName)
		if displayName.Valid {
			dev.DisplayName = displayName.String
		}
	}
	return &dev, err
}

func (s *devicesStatements) selectDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error) {
	rows, err := s.selectDevicesByIDStmt.QueryContext(ctx, pq.StringArray(deviceIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectDevicesByID: rows.close() failed")
	var devices []api.Device
	for rows.Next() {
		var dev api.Device
		var localpart string
		var displayName sql.NullString
		if err := rows.Scan(&dev.ID, &localpart, &displayName); err != nil {
			return nil, err
		}
		if displayName.Valid {
			dev.DisplayName = displayName.String
		}
		dev.UserID = userutil.MakeUserID(localpart, s.serverName)
		devices = append(devices, dev)
	}
	return devices, rows.Err()
}

func (s *devicesStatements) selectDevicesByLocalpart(
	ctx context.Context, txn *sql.Tx, localpart, exceptDeviceID string,
) ([]api.Device, error) {
	devices := []api.Device{}
	rows, err := sqlutil.TxStmt(txn, s.selectDevicesByLocalpartStmt).QueryContext(ctx, localpart, exceptDeviceID)

	if err != nil {
		return devices, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectDevicesByLocalpart: rows.close() failed")

	for rows.Next() {
		var dev api.Device
		var id, displayname sql.NullString
		err = rows.Scan(&id, &displayname)
		if err != nil {
			return devices, err
		}
		if id.Valid {
			dev.ID = id.String
		}
		if displayname.Valid {
			dev.DisplayName = displayname.String
		}
		dev.UserID = userutil.MakeUserID(localpart, s.serverName)
		devices = append(devices, dev)
	}

	return devices, rows.Err()
}

func (s *devicesStatements) updateDeviceLastSeen(ctx context.Context, txn *sql.Tx, deviceID, ipAddr string) error {
	lastSeenTs := time.Now().UnixNano() / 1000000
	stmt := sqlutil.TxStmt(txn, s.updateDeviceLastSeenStmt)
	_, err := stmt.ExecContext(ctx, lastSeenTs, ipAddr, deviceID)
	return err
}
