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

package devices

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/common"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrixserverlib"
)

const devicesSchema = `
-- Stores data about devices.
CREATE TABLE IF NOT EXISTS device_devices (
    -- The access token granted to this device. This has to be the primary key
    -- so we can distinguish which device is making a given request.
    access_token TEXT NOT NULL PRIMARY KEY,
    -- The device identifier. This only needs to uniquely identify a device for a given user, not globally.
    -- access_tokens will be clobbered based on the device ID for a user.
    device_id TEXT NOT NULL,
    -- The Matrix user ID localpart for this device. This is preferable to storing the full user_id
    -- as it is smaller, makes it clearer that we only manage devices for our own users, and may make
    -- migration to different domain names easier.
    localpart TEXT NOT NULL,
    -- When this devices was first recognised on the network, as a unix timestamp (ms resolution).
    created_ts BIGINT NOT NULL
    -- TODO: device keys, device display names, last used ts and IP address?, token restrictions (if 3rd-party OAuth app)
);

-- Device IDs must be unique for a given user.
CREATE UNIQUE INDEX IF NOT EXISTS device_localpart_id_idx ON device_devices(localpart, device_id);
`

const insertDeviceSQL = "" +
	"INSERT INTO device_devices(device_id, localpart, access_token, created_ts) VALUES ($1, $2, $3, $4)"

const selectDeviceByTokenSQL = "" +
	"SELECT device_id, localpart FROM device_devices WHERE access_token = $1"

const deleteDeviceSQL = "" +
	"DELETE FROM device_devices WHERE device_id = $1 AND localpart = $2"

// TODO: List devices?

type devicesStatements struct {
	insertDeviceStmt        *sql.Stmt
	selectDeviceByTokenStmt *sql.Stmt
	deleteDeviceStmt        *sql.Stmt
	serverName              gomatrixserverlib.ServerName
}

func (s *devicesStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
	_, err = db.Exec(devicesSchema)
	if err != nil {
		return
	}
	if s.insertDeviceStmt, err = db.Prepare(insertDeviceSQL); err != nil {
		return
	}
	if s.selectDeviceByTokenStmt, err = db.Prepare(selectDeviceByTokenSQL); err != nil {
		return
	}
	if s.deleteDeviceStmt, err = db.Prepare(deleteDeviceSQL); err != nil {
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
) (*authtypes.Device, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	stmt := common.TxStmt(txn, s.insertDeviceStmt)
	if _, err := stmt.ExecContext(ctx, id, localpart, accessToken, createdTimeMS); err != nil {
		return nil, err
	}
	return &authtypes.Device{
		ID:          id,
		UserID:      makeUserID(localpart, s.serverName),
		AccessToken: accessToken,
	}, nil
}

func (s *devicesStatements) deleteDevice(
	ctx context.Context, txn *sql.Tx, id, localpart string,
) error {
	stmt := common.TxStmt(txn, s.deleteDeviceStmt)
	_, err := stmt.ExecContext(ctx, id, localpart)
	return err
}

func (s *devicesStatements) selectDeviceByToken(
	ctx context.Context, accessToken string,
) (*authtypes.Device, error) {
	var dev authtypes.Device
	var localpart string
	stmt := s.selectDeviceByTokenStmt
	err := stmt.QueryRowContext(ctx, accessToken).Scan(&dev.ID, &localpart)
	if err == nil {
		dev.UserID = makeUserID(localpart, s.serverName)
		dev.AccessToken = accessToken
	}
	return &dev, err
}

func makeUserID(localpart string, server gomatrixserverlib.ServerName) string {
	return fmt.Sprintf("@%s:%s", localpart, string(server))
}
