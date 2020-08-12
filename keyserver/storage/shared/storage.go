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
	"github.com/matrix-org/gomatrixserverlib"
)

type Database struct {
	DB                    *sql.DB
	OneTimeKeysTable      tables.OneTimeKeys
	DeviceKeysTable       tables.DeviceKeys
	KeyChangesTable       tables.KeyChanges
	StaleDeviceListsTable tables.StaleDeviceLists
}

func (d *Database) ExistingOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error) {
	return d.OneTimeKeysTable.SelectOneTimeKeys(ctx, userID, deviceID, keyIDsWithAlgorithms)
}

func (d *Database) StoreOneTimeKeys(ctx context.Context, keys api.OneTimeKeys) (*api.OneTimeKeysCount, error) {
	return d.OneTimeKeysTable.InsertOneTimeKeys(ctx, keys)
}

func (d *Database) OneTimeKeysCount(ctx context.Context, userID, deviceID string) (*api.OneTimeKeysCount, error) {
	return d.OneTimeKeysTable.CountOneTimeKeys(ctx, userID, deviceID)
}

func (d *Database) DeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error {
	return d.DeviceKeysTable.SelectDeviceKeysJSON(ctx, keys)
}

func (d *Database) PrevIDsExists(ctx context.Context, userID string, prevIDs []int) (bool, error) {
	sids := make([]int64, len(prevIDs))
	for i := range prevIDs {
		sids[i] = int64(prevIDs[i])
	}
	count, err := d.DeviceKeysTable.CountStreamIDsForUser(ctx, userID, sids)
	if err != nil {
		return false, err
	}
	return count == len(prevIDs), nil
}

func (d *Database) StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage, clearUserIDs []string) error {
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		for _, userID := range clearUserIDs {
			err := d.DeviceKeysTable.DeleteAllDeviceKeys(ctx, txn, userID)
			if err != nil {
				return err
			}
		}
		return d.DeviceKeysTable.InsertDeviceKeys(ctx, txn, keys)
	})
}

func (d *Database) StoreLocalDeviceKeys(ctx context.Context, keys []api.DeviceMessage) error {
	// work out the latest stream IDs for each user
	userIDToStreamID := make(map[string]int)
	for _, k := range keys {
		userIDToStreamID[k.UserID] = 0
	}
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		for userID := range userIDToStreamID {
			streamID, err := d.DeviceKeysTable.SelectMaxStreamIDForUser(ctx, txn, userID)
			if err != nil {
				return err
			}
			userIDToStreamID[userID] = int(streamID)
		}
		// set the stream IDs for each key
		for i := range keys {
			k := keys[i]
			userIDToStreamID[k.UserID]++ // start stream from 1
			k.StreamID = userIDToStreamID[k.UserID]
			keys[i] = k
		}
		return d.DeviceKeysTable.InsertDeviceKeys(ctx, txn, keys)
	})
}

func (d *Database) DeviceKeysForUser(ctx context.Context, userID string, deviceIDs []string) ([]api.DeviceMessage, error) {
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

func (d *Database) StoreKeyChange(ctx context.Context, partition int32, offset int64, userID string) error {
	return d.KeyChangesTable.InsertKeyChange(ctx, partition, offset, userID)
}

func (d *Database) KeyChanges(ctx context.Context, partition int32, fromOffset, toOffset int64) (userIDs []string, latestOffset int64, err error) {
	return d.KeyChangesTable.SelectKeyChanges(ctx, partition, fromOffset, toOffset)
}

// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
// If no domains are given, all user IDs with stale device lists are returned.
func (d *Database) StaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error) {
	return d.StaleDeviceListsTable.SelectUserIDsWithStaleDeviceLists(ctx, domains)
}

// MarkDeviceListStale sets the stale bit for this user to isStale.
func (d *Database) MarkDeviceListStale(ctx context.Context, userID string, isStale bool) error {
	return d.StaleDeviceListsTable.InsertStaleDeviceList(ctx, userID, isStale)
}
