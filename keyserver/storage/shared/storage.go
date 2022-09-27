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
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database struct {
	DB                    *sql.DB
	Writer                sqlutil.Writer
	OneTimeKeysTable      tables.OneTimeKeys
	DeviceKeysTable       tables.DeviceKeys
	KeyChangesTable       tables.KeyChanges
	StaleDeviceListsTable tables.StaleDeviceLists
	CrossSigningKeysTable tables.CrossSigningKeys
	CrossSigningSigsTable tables.CrossSigningSigs
}

func (d *Database) ExistingOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error) {
	return d.OneTimeKeysTable.SelectOneTimeKeys(ctx, userID, deviceID, keyIDsWithAlgorithms)
}

func (d *Database) StoreOneTimeKeys(ctx context.Context, keys api.OneTimeKeys) (counts *api.OneTimeKeysCount, err error) {
	_ = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		counts, err = d.OneTimeKeysTable.InsertOneTimeKeys(ctx, txn, keys)
		return err
	})
	return
}

func (d *Database) OneTimeKeysCount(ctx context.Context, userID, deviceID string) (*api.OneTimeKeysCount, error) {
	return d.OneTimeKeysTable.CountOneTimeKeys(ctx, userID, deviceID)
}

func (d *Database) DeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error {
	return d.DeviceKeysTable.SelectDeviceKeysJSON(ctx, keys)
}

func (d *Database) PrevIDsExists(ctx context.Context, userID string, prevIDs []int64) (bool, error) {
	count, err := d.DeviceKeysTable.CountStreamIDsForUser(ctx, userID, prevIDs)
	if err != nil {
		return false, err
	}
	return count == len(prevIDs), nil
}

func (d *Database) StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage, clearUserIDs []string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
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
	userIDToStreamID := make(map[string]int64)
	for _, k := range keys {
		userIDToStreamID[k.UserID] = 0
	}
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		for userID := range userIDToStreamID {
			streamID, err := d.DeviceKeysTable.SelectMaxStreamIDForUser(ctx, txn, userID)
			if err != nil {
				return err
			}
			userIDToStreamID[userID] = streamID
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

func (d *Database) DeviceKeysForUser(ctx context.Context, userID string, deviceIDs []string, includeEmpty bool) ([]api.DeviceMessage, error) {
	return d.DeviceKeysTable.SelectBatchDeviceKeys(ctx, userID, deviceIDs, includeEmpty)
}

func (d *Database) ClaimKeys(ctx context.Context, userToDeviceToAlgorithm map[string]map[string]string) ([]api.OneTimeKeys, error) {
	var result []api.OneTimeKeys
	err := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
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

func (d *Database) StoreKeyChange(ctx context.Context, userID string) (id int64, err error) {
	err = d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		id, err = d.KeyChangesTable.InsertKeyChange(ctx, userID)
		return err
	})
	return
}

func (d *Database) KeyChanges(ctx context.Context, fromOffset, toOffset int64) (userIDs []string, latestOffset int64, err error) {
	return d.KeyChangesTable.SelectKeyChanges(ctx, fromOffset, toOffset)
}

// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
// If no domains are given, all user IDs with stale device lists are returned.
func (d *Database) StaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error) {
	return d.StaleDeviceListsTable.SelectUserIDsWithStaleDeviceLists(ctx, domains)
}

// MarkDeviceListStale sets the stale bit for this user to isStale.
func (d *Database) MarkDeviceListStale(ctx context.Context, userID string, isStale bool) error {
	return d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		return d.StaleDeviceListsTable.InsertStaleDeviceList(ctx, userID, isStale)
	})
}

// DeleteDeviceKeys removes the device keys for a given user/device, and any accompanying
// cross-signing signatures relating to that device.
func (d *Database) DeleteDeviceKeys(ctx context.Context, userID string, deviceIDs []gomatrixserverlib.KeyID) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		for _, deviceID := range deviceIDs {
			if err := d.CrossSigningSigsTable.DeleteCrossSigningSigsForTarget(ctx, txn, userID, deviceID); err != nil && err != sql.ErrNoRows {
				return fmt.Errorf("d.CrossSigningSigsTable.DeleteCrossSigningSigsForTarget: %w", err)
			}
			if err := d.DeviceKeysTable.DeleteDeviceKeys(ctx, txn, userID, string(deviceID)); err != nil && err != sql.ErrNoRows {
				return fmt.Errorf("d.DeviceKeysTable.DeleteDeviceKeys: %w", err)
			}
			if err := d.OneTimeKeysTable.DeleteOneTimeKeys(ctx, txn, userID, string(deviceID)); err != nil && err != sql.ErrNoRows {
				return fmt.Errorf("d.OneTimeKeysTable.DeleteOneTimeKeys: %w", err)
			}
		}
		return nil
	})
}

// CrossSigningKeysForUser returns the latest known cross-signing keys for a user, if any.
func (d *Database) CrossSigningKeysForUser(ctx context.Context, userID string) (map[gomatrixserverlib.CrossSigningKeyPurpose]gomatrixserverlib.CrossSigningKey, error) {
	keyMap, err := d.CrossSigningKeysTable.SelectCrossSigningKeysForUser(ctx, nil, userID)
	if err != nil {
		return nil, fmt.Errorf("d.CrossSigningKeysTable.SelectCrossSigningKeysForUser: %w", err)
	}
	results := map[gomatrixserverlib.CrossSigningKeyPurpose]gomatrixserverlib.CrossSigningKey{}
	for purpose, key := range keyMap {
		keyID := gomatrixserverlib.KeyID("ed25519:" + key.Encode())
		result := gomatrixserverlib.CrossSigningKey{
			UserID: userID,
			Usage:  []gomatrixserverlib.CrossSigningKeyPurpose{purpose},
			Keys: map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{
				keyID: key,
			},
		}
		sigMap, err := d.CrossSigningSigsTable.SelectCrossSigningSigsForTarget(ctx, nil, userID, userID, keyID)
		if err != nil {
			continue
		}
		for sigUserID, forSigUserID := range sigMap {
			if userID != sigUserID {
				continue
			}
			if result.Signatures == nil {
				result.Signatures = map[string]map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{}
			}
			if _, ok := result.Signatures[sigUserID]; !ok {
				result.Signatures[sigUserID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{}
			}
			for sigKeyID, sigBytes := range forSigUserID {
				result.Signatures[sigUserID][sigKeyID] = sigBytes
			}
		}
		results[purpose] = result
	}
	return results, nil
}

// CrossSigningKeysForUser returns the latest known cross-signing keys for a user, if any.
func (d *Database) CrossSigningKeysDataForUser(ctx context.Context, userID string) (types.CrossSigningKeyMap, error) {
	return d.CrossSigningKeysTable.SelectCrossSigningKeysForUser(ctx, nil, userID)
}

// CrossSigningSigsForTarget returns the signatures for a given user's key ID, if any.
func (d *Database) CrossSigningSigsForTarget(ctx context.Context, originUserID, targetUserID string, targetKeyID gomatrixserverlib.KeyID) (types.CrossSigningSigMap, error) {
	return d.CrossSigningSigsTable.SelectCrossSigningSigsForTarget(ctx, nil, originUserID, targetUserID, targetKeyID)
}

// StoreCrossSigningKeysForUser stores the latest known cross-signing keys for a user.
func (d *Database) StoreCrossSigningKeysForUser(ctx context.Context, userID string, keyMap types.CrossSigningKeyMap) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		for keyType, keyData := range keyMap {
			if err := d.CrossSigningKeysTable.UpsertCrossSigningKeysForUser(ctx, txn, userID, keyType, keyData); err != nil {
				return fmt.Errorf("d.CrossSigningKeysTable.InsertCrossSigningKeysForUser: %w", err)
			}
		}
		return nil
	})
}

// StoreCrossSigningSigsForTarget stores a signature for a target user ID and key/dvice.
func (d *Database) StoreCrossSigningSigsForTarget(
	ctx context.Context,
	originUserID string, originKeyID gomatrixserverlib.KeyID,
	targetUserID string, targetKeyID gomatrixserverlib.KeyID,
	signature gomatrixserverlib.Base64Bytes,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if err := d.CrossSigningSigsTable.UpsertCrossSigningSigsForTarget(ctx, nil, originUserID, originKeyID, targetUserID, targetKeyID, signature); err != nil {
			return fmt.Errorf("d.CrossSigningSigsTable.InsertCrossSigningSigsForTarget: %w", err)
		}
		return nil
	})
}
