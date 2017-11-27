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

const insertDeviceSQL = "" +
	"INSERT INTO device_devices(device_id, localpart, access_token, created_ts, display_name) VALUES ($1, $2, $3, $4, $5)"

const selectDeviceByTokenSQL = "" +
	"SELECT device_id, localpart FROM device_devices WHERE access_token = $1"

const selectDeviceByIDSQL = "" +
	"SELECT display_name FROM device_devices WHERE localpart = $1 and device_id = $2"

const selectDevicesByLocalpartSQL = "" +
	"SELECT device_id, display_name FROM device_devices WHERE localpart = $1"

const updateDeviceNameSQL = "" +
	"UPDATE device_devices SET display_name = $1 WHERE localpart = $2 AND device_id = $3"

const deleteDeviceSQL = "" +
	"DELETE FROM device_devices WHERE device_id = $1 AND localpart = $2"

const deleteDevicesByLocalpartSQL = "" +
	"DELETE FROM device_devices WHERE localpart = $1"

type devicesStatements struct {
	insertDeviceStmt             *sql.Stmt
	selectDeviceByTokenStmt      *sql.Stmt
	selectDeviceByIDStmt         *sql.Stmt
	selectDevicesByLocalpartStmt *sql.Stmt
	updateDeviceNameStmt         *sql.Stmt
	deleteDeviceStmt             *sql.Stmt
	deleteDevicesByLocalpartStmt *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
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
	s.serverName = server
	return
}

// insertDevice creates a new device. Returns an error if any device with the same access token already exists.
// Returns an error if the user already has a device with the given device ID.
// Returns the device on success.
func (s *devicesStatements) insertDevice(
	ctx context.Context, txn *sql.Tx, id, localpart, accessToken string,
	displayName *string,
) (*authtypes.Device, error) {
	createdTimeMS := time.Now().UnixNano() / 1000000
	stmt := common.TxStmt(txn, s.insertDeviceStmt)
	if _, err := stmt.ExecContext(ctx, id, localpart, accessToken, createdTimeMS, displayName); err != nil {
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

func (s *devicesStatements) deleteDevicesByLocalpart(
	ctx context.Context, txn *sql.Tx, localpart string,
) error {
	stmt := common.TxStmt(txn, s.deleteDevicesByLocalpartStmt)
	_, err := stmt.ExecContext(ctx, localpart)
	return err
}

func (s *devicesStatements) updateDeviceName(
	ctx context.Context, txn *sql.Tx, localpart, deviceID string, displayName *string,
) error {
	stmt := common.TxStmt(txn, s.updateDeviceNameStmt)
	_, err := stmt.ExecContext(ctx, displayName, localpart, deviceID)
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

func (s *devicesStatements) selectDeviceByID(
	ctx context.Context, localpart, deviceID string,
) (*authtypes.Device, error) {
	var dev authtypes.Device
	var created int64
	stmt := s.selectDeviceByIDStmt
	err := stmt.QueryRowContext(ctx, localpart, deviceID).Scan(&created)
	if err == nil {
		dev.ID = deviceID
		dev.UserID = makeUserID(localpart, s.serverName)
	}
	return &dev, err
}

func (s *devicesStatements) selectDevicesByLocalpart(
	ctx context.Context, localpart string,
) ([]authtypes.Device, error) {
	devices := []authtypes.Device{}

	rows, err := s.selectDevicesByLocalpartStmt.QueryContext(ctx, localpart)

	if err != nil {
		return devices, err
	}

	for rows.Next() {
		var dev authtypes.Device
		err = rows.Scan(&dev.ID)
		if err != nil {
			return devices, err
		}
		dev.UserID = makeUserID(localpart, s.serverName)
		devices = append(devices, dev)
	}

	return devices, nil
}

func makeUserID(localpart string, server gomatrixserverlib.ServerName) string {
	return fmt.Sprintf("@%s:%s", localpart, string(server))
}
