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
	"database/sql"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrixserverlib"
)

const devicesSchema = `
-- Stores data about devices.
CREATE TABLE IF NOT EXISTS devices (
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
CREATE UNIQUE INDEX IF NOT EXISTS localpart_id_idx ON devices(localpart, device_id);
`

const insertDeviceSQL = "" +
	"INSERT INTO devices(device_id, localpart, access_token, created_ts) VALUES ($1, $2, $3, $4)"

const selectDeviceByTokenSQL = "" +
	"SELECT device_id, localpart FROM devices WHERE access_token = $1"

const deleteDeviceSQL = "" +
	"DELETE FROM devices WHERE device_id = $1 AND localpart = $2"

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
func (s *devicesStatements) insertDevice(txn *sql.Tx, id, localpart, accessToken string) (dev *authtypes.Device, err error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	if _, err = txn.Stmt(s.insertDeviceStmt).Exec(id, localpart, accessToken, createdTimeMS); err == nil {
		dev = &authtypes.Device{
			ID:          id,
			UserID:      makeUserID(localpart, s.serverName),
			AccessToken: accessToken,
		}
	}
	return
}

func (s *devicesStatements) deleteDevice(txn *sql.Tx, id, localpart string) error {
	_, err := txn.Stmt(s.deleteDeviceStmt).Exec(id, localpart)
	return err
}

func (s *devicesStatements) selectDeviceByToken(accessToken string) (*authtypes.Device, error) {
	var dev authtypes.Device
	var localpart string
	err := s.selectDeviceByTokenStmt.QueryRow(accessToken).Scan(&dev.ID, &localpart)
	if err == nil {
		dev.UserID = makeUserID(localpart, s.serverName)
		dev.AccessToken = accessToken
	}
	return &dev, err
}

func makeUserID(localpart string, server gomatrixserverlib.ServerName) string {
	return fmt.Sprintf("@%s:%s", localpart, string(server))
}
