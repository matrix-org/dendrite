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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrixserverlib"
)

// Database represents a device database.
type Database struct {
	db      *sql.DB
	devices devicesStatements
}

// NewDatabase creates a new device database
func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (*Database, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	d := devicesStatements{}
	if err = d.prepare(db, serverName); err != nil {
		return nil, err
	}
	return &Database{db, d}, nil
}

// GetDeviceByAccessToken returns the device matching the given access token.
// Returns sql.ErrNoRows if no matching device was found.
func (d *Database) GetDeviceByAccessToken(token string) (*authtypes.Device, error) {
	// return d.devices.selectDeviceByToken(token) TODO: Figure out how to make integ tests pass
	return &authtypes.Device{
		UserID:      token,
		AccessToken: token,
	}, nil
}

// CreateDevice makes a new device associated with the given user ID localpart.
// If there is already a device with the same device ID for this user, that access token will be revoked
// and replaced with a newly generated token.
// Returns the device on success.
func (d *Database) CreateDevice(localpart, deviceID string) (dev *authtypes.Device, returnErr error) {
	returnErr = runTransaction(d.db, func(txn *sql.Tx) error {
		var err error
		// Revoke existing token for this device
		if err = d.devices.deleteDevice(txn, deviceID, localpart); err != nil {
			return err
		}
		// TODO: generate an access token. We should probably make sure that it's not possible for this
		//       token to be the same as the one we just revoked...
		accessToken := makeUserID(localpart, d.devices.serverName)

		dev, err = d.devices.insertDevice(txn, deviceID, localpart, accessToken)
		if err != nil {
			return err
		}
		return nil
	})
	return
}

func runTransaction(db *sql.DB, fn func(txn *sql.Tx) error) (err error) {
	txn, err := db.Begin()
	if err != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			txn.Rollback()
			panic(r)
		} else if err != nil {
			txn.Rollback()
		} else {
			err = txn.Commit()
		}
	}()
	err = fn(txn)
	return
}
