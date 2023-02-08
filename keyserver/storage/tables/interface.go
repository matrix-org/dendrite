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

package tables

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type OneTimeKeys interface {
	SelectOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error)
	CountOneTimeKeys(ctx context.Context, userID, deviceID string) (*api.OneTimeKeysCount, error)
	InsertOneTimeKeys(ctx context.Context, txn *sql.Tx, keys api.OneTimeKeys) (*api.OneTimeKeysCount, error)
	// SelectAndDeleteOneTimeKey selects a single one time key matching the user/device/algorithm specified and returns the algo:key_id => JSON.
	// Returns an empty map if the key does not exist.
	SelectAndDeleteOneTimeKey(ctx context.Context, txn *sql.Tx, userID, deviceID, algorithm string) (map[string]json.RawMessage, error)
	DeleteOneTimeKeys(ctx context.Context, txn *sql.Tx, userID, deviceID string) error
}

type DeviceKeys interface {
	SelectDeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error
	InsertDeviceKeys(ctx context.Context, txn *sql.Tx, keys []api.DeviceMessage) error
	SelectMaxStreamIDForUser(ctx context.Context, txn *sql.Tx, userID string) (streamID int64, err error)
	CountStreamIDsForUser(ctx context.Context, userID string, streamIDs []int64) (int, error)
	SelectBatchDeviceKeys(ctx context.Context, userID string, deviceIDs []string, includeEmpty bool) ([]api.DeviceMessage, error)
	DeleteDeviceKeys(ctx context.Context, txn *sql.Tx, userID, deviceID string) error
	DeleteAllDeviceKeys(ctx context.Context, txn *sql.Tx, userID string) error
}

type KeyChanges interface {
	InsertKeyChange(ctx context.Context, userID string) (int64, error)
	// SelectKeyChanges returns the set (de-duplicated) of users who have changed their keys between the two offsets.
	// Results are exclusive of fromOffset and inclusive of toOffset. A toOffset of types.OffsetNewest means no upper offset.
	SelectKeyChanges(ctx context.Context, fromOffset, toOffset int64) (userIDs []string, latestOffset int64, err error)

	Prepare() error
}

type StaleDeviceLists interface {
	InsertStaleDeviceList(ctx context.Context, userID string, isStale bool) error
	SelectUserIDsWithStaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error)
}

type CrossSigningKeys interface {
	SelectCrossSigningKeysForUser(ctx context.Context, txn *sql.Tx, userID string) (r types.CrossSigningKeyMap, err error)
	UpsertCrossSigningKeysForUser(ctx context.Context, txn *sql.Tx, userID string, keyType gomatrixserverlib.CrossSigningKeyPurpose, keyData gomatrixserverlib.Base64Bytes) error
}

type CrossSigningSigs interface {
	SelectCrossSigningSigsForTarget(ctx context.Context, txn *sql.Tx, originUserID, targetUserID string, targetKeyID gomatrixserverlib.KeyID) (r types.CrossSigningSigMap, err error)
	UpsertCrossSigningSigsForTarget(ctx context.Context, txn *sql.Tx, originUserID string, originKeyID gomatrixserverlib.KeyID, targetUserID string, targetKeyID gomatrixserverlib.KeyID, signature gomatrixserverlib.Base64Bytes) error
	DeleteCrossSigningSigsForTarget(ctx context.Context, txn *sql.Tx, targetUserID string, targetKeyID gomatrixserverlib.KeyID) error
}
