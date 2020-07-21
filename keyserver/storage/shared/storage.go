// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package shared

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

type Database struct {
	DB               *sql.DB
	OneTimeKeysTable tables.OneTimeKeys
	DeviceKeysTable  tables.DeviceKeys
}

func (d *Database) ExistingOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error) {
	return d.OneTimeKeysTable.SelectOneTimeKeys(ctx, userID, deviceID, keyIDsWithAlgorithms)
}

func (d *Database) StoreOneTimeKeys(ctx context.Context, keys api.OneTimeKeys) (*api.OneTimeKeysCount, error) {
	return d.OneTimeKeysTable.InsertOneTimeKeys(ctx, keys)
}

func (d *Database) DeviceKeysJSON(ctx context.Context, keys []api.DeviceKeys) error {
	return d.DeviceKeysTable.SelectDeviceKeysJSON(ctx, keys)
}

func (d *Database) StoreDeviceKeys(ctx context.Context, keys []api.DeviceKeys) error {
	return d.DeviceKeysTable.InsertDeviceKeys(ctx, keys)
}

func (d *Database) DeviceKeysForUser(ctx context.Context, userID string, deviceIDs []string) ([]api.DeviceKeys, error) {
	return d.DeviceKeysTable.SelectBatchDeviceKeys(ctx, userID, deviceIDs)
}

func (d *Database) ClaimKeys(ctx context.Context, userToDeviceToAlgorithm map[string]map[string]string) ([]api.OneTimeKeys, error) {
	var result []api.OneTimeKeys
	err := sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		for userID, deviceToAlgo := range userToDeviceToAlgorithm {
			for deviceID, algo := range deviceToAlgo {
				keyJSON, err := d.OneTimeKeysTable.SelectAndDeleteOneTimeKey(ctx, txn, userID, deviceID, algo)
				if err != nil {
					return err
				}
				if keyJSON != nil {
					result = append(result, api.OneTimeKeys{
						UserID:   userID,
						DeviceID: deviceID,
						KeyJSON:  keyJSON,
					})
				}
			}
		}
		return nil
	})
	return result, err
}
