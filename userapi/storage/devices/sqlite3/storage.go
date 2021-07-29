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
	"crypto/rand"
	"database/sql"
	"encoding/base64"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/devices/sqlite3/deltas"
	"github.com/matrix-org/gomatrixserverlib"
)

// The length of generated device IDs
var deviceIDByteLength = 6

// Database represents a device database.
type Database struct {
	db      *sql.DB
	writer  sqlutil.Writer
	devices devicesStatements
}

// NewDatabase creates a new device database
func NewDatabase(dbProperties *config.DatabaseOptions, serverName gomatrixserverlib.ServerName) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	writer := sqlutil.NewExclusiveWriter()
	d := devicesStatements{}

	// Create tables before executing migrations so we don't fail if the table is missing,
	// and THEN prepare statements so we don't fail due to referencing new columns
	if err = d.execSchema(db); err != nil {
		return nil, err
	}
	m := sqlutil.NewMigrations()
	deltas.LoadLastSeenTSIP(m)
	if err = m.RunDeltas(db, dbProperties); err != nil {
		return nil, err
	}
	if err = d.prepare(db, writer, serverName); err != nil {
		return nil, err
	}
	return &Database{db, writer, d}, nil
}

// GetDeviceByAccessToken returns the device matching the given access token.
// Returns sql.ErrNoRows if no matching device was found.
func (d *Database) GetDeviceByAccessToken(
	ctx context.Context, token string,
) (*api.Device, error) {
	return d.devices.selectDeviceByToken(ctx, token)
}

// GetDeviceByID returns the device matching the given ID.
// Returns sql.ErrNoRows if no matching device was found.
func (d *Database) GetDeviceByID(
	ctx context.Context, localpart, deviceID string,
) (*api.Device, error) {
	return d.devices.selectDeviceByID(ctx, localpart, deviceID)
}

// GetDevicesByLocalpart returns the devices matching the given localpart.
func (d *Database) GetDevicesByLocalpart(
	ctx context.Context, localpart string,
) ([]api.Device, error) {
	return d.devices.selectDevicesByLocalpart(ctx, nil, localpart, "")
}

func (d *Database) GetDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error) {
	return d.devices.selectDevicesByID(ctx, deviceIDs)
}

// CreateDevice makes a new device associated with the given user ID localpart.
// If there is already a device with the same device ID for this user, that access token will be revoked
// and replaced with the given accessToken. If the given accessToken is already in use for another device,
// an error will be returned.
// If no device ID is given one is generated.
// Returns the device on success.
func (d *Database) CreateDevice(
	ctx context.Context, localpart string, deviceID *string, accessToken string,
	displayName *string, ipAddr, userAgent string,
) (dev *api.Device, returnErr error) {
	if deviceID != nil {
		returnErr = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
			var err error
			// Revoke existing tokens for this device
			if err = d.devices.deleteDevice(ctx, txn, *deviceID, localpart); err != nil {
				return err
			}

			dev, err = d.devices.insertDevice(ctx, txn, *deviceID, localpart, accessToken, displayName, ipAddr, userAgent)
			return err
		})
	} else {
		// We generate device IDs in a loop in case its already taken.
		// We cap this at going round 5 times to ensure we don't spin forever
		var newDeviceID string
		for i := 1; i <= 5; i++ {
			newDeviceID, returnErr = generateDeviceID()
			if returnErr != nil {
				return
			}

			returnErr = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
				var err error
				dev, err = d.devices.insertDevice(ctx, txn, newDeviceID, localpart, accessToken, displayName, ipAddr, userAgent)
				return err
			})
			if returnErr == nil {
				return
			}
		}
	}
	return
}

// generateDeviceID creates a new device id. Returns an error if failed to generate
// random bytes.
func generateDeviceID() (string, error) {
	b := make([]byte, deviceIDByteLength)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	// url-safe no padding
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// UpdateDevice updates the given device with the display name.
// Returns SQL error if there are problems and nil on success.
func (d *Database) UpdateDevice(
	ctx context.Context, localpart, deviceID string, displayName *string,
) error {
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.devices.updateDeviceName(ctx, txn, localpart, deviceID, displayName)
	})
}

// RemoveDevice revokes a device by deleting the entry in the database
// matching with the given device ID and user ID localpart.
// If the device doesn't exist, it will not return an error
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemoveDevice(
	ctx context.Context, deviceID, localpart string,
) error {
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		if err := d.devices.deleteDevice(ctx, txn, deviceID, localpart); err != sql.ErrNoRows {
			return err
		}
		return nil
	})
}

// RemoveDevices revokes one or more devices by deleting the entry in the database
// matching with the given device IDs and user ID localpart.
// If the devices don't exist, it will not return an error
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemoveDevices(
	ctx context.Context, localpart string, devices []string,
) error {
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		if err := d.devices.deleteDevices(ctx, txn, localpart, devices); err != sql.ErrNoRows {
			return err
		}
		return nil
	})
}

// RemoveAllDevices revokes devices by deleting the entry in the
// database matching the given user ID localpart.
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemoveAllDevices(
	ctx context.Context, localpart, exceptDeviceID string,
) (devices []api.Device, err error) {
	err = d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		devices, err = d.devices.selectDevicesByLocalpart(ctx, txn, localpart, exceptDeviceID)
		if err != nil {
			return err
		}
		if err := d.devices.deleteDevicesByLocalpart(ctx, txn, localpart, exceptDeviceID); err != sql.ErrNoRows {
			return err
		}
		return nil
	})
	return
}

// UpdateDeviceLastSeen updates a the last seen timestamp and the ip address
func (d *Database) UpdateDeviceLastSeen(ctx context.Context, localpart, deviceID, ipAddr string) error {
	return d.writer.Do(d.db, nil, func(txn *sql.Tx) error {
		return d.devices.updateDeviceLastSeen(ctx, txn, localpart, deviceID, ipAddr)
	})
}
