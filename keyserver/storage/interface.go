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

package storage

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	// ExistingOneTimeKeys returns a map of keyIDWithAlgorithm to key JSON for the given parameters. If no keys exist with this combination
	// of user/device/key/algorithm 4-uple then it is omitted from the map. Returns an error when failing to communicate with the database.
	ExistingOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error)

	// StoreOneTimeKeys persists the given one-time keys.
	StoreOneTimeKeys(ctx context.Context, keys api.OneTimeKeys) (*api.OneTimeKeysCount, error)

	// OneTimeKeysCount returns a count of all OTKs for this device.
	OneTimeKeysCount(ctx context.Context, userID, deviceID string) (*api.OneTimeKeysCount, error)

	// DeviceKeysJSON populates the KeyJSON for the given keys. If any proided `keys` have a `KeyJSON` or `StreamID` already then it will be replaced.
	DeviceKeysJSON(ctx context.Context, keys []api.DeviceMessage) error

	// StoreLocalDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced. An empty KeyJSON removes the key
	// for this (user, device).
	// The `StreamID` for each message is set on successful insertion. In the event the key already exists, the existing StreamID is set.
	// Returns an error if there was a problem storing the keys.
	StoreLocalDeviceKeys(ctx context.Context, keys []api.DeviceMessage) error

	// StoreRemoteDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced. An empty KeyJSON removes the key
	// for this (user, device). Does not modify the stream ID for keys. User IDs in `clearUserIDs` will have all their device keys deleted prior
	// to insertion - use this when you have a complete snapshot of a user's keys in order to track device deletions correctly.
	StoreRemoteDeviceKeys(ctx context.Context, keys []api.DeviceMessage, clearUserIDs []string) error

	// PrevIDsExists returns true if all prev IDs exist for this user.
	PrevIDsExists(ctx context.Context, userID string, prevIDs []int64) (bool, error)

	// DeviceKeysForUser returns the device keys for the device IDs given. If the length of deviceIDs is 0, all devices are selected.
	// If there are some missing keys, they are omitted from the returned slice. There is no ordering on the returned slice.
	DeviceKeysForUser(ctx context.Context, userID string, deviceIDs []string, includeEmpty bool) ([]api.DeviceMessage, error)

	// DeleteDeviceKeys removes the device keys for a given user/device, and any accompanying
	// cross-signing signatures relating to that device.
	DeleteDeviceKeys(ctx context.Context, userID string, deviceIDs []gomatrixserverlib.KeyID) error

	// ClaimKeys based on the 3-uple of user_id, device_id and algorithm name. Returns the keys claimed. Returns no error if a key
	// cannot be claimed or if none exist for this (user, device, algorithm), instead it is omitted from the returned slice.
	ClaimKeys(ctx context.Context, userToDeviceToAlgorithm map[string]map[string]string) ([]api.OneTimeKeys, error)

	// StoreKeyChange stores key change metadata and returns the device change ID which represents the position in the /sync stream for this device change.
	// `userID` is the the user who has changed their keys in some way.
	StoreKeyChange(ctx context.Context, userID string) (int64, error)

	// KeyChanges returns a list of user IDs who have modified their keys from the offset given (exclusive) to the offset given (inclusive).
	// A to offset of types.OffsetNewest means no upper limit.
	// Returns the offset of the latest key change.
	KeyChanges(ctx context.Context, fromOffset, toOffset int64) (userIDs []string, latestOffset int64, err error)

	// StaleDeviceLists returns a list of user IDs ending with the domains provided who have stale device lists.
	// If no domains are given, all user IDs with stale device lists are returned.
	StaleDeviceLists(ctx context.Context, domains []gomatrixserverlib.ServerName) ([]string, error)

	// MarkDeviceListStale sets the stale bit for this user to isStale.
	MarkDeviceListStale(ctx context.Context, userID string, isStale bool) error

	CrossSigningKeysForUser(ctx context.Context, userID string) (map[gomatrixserverlib.CrossSigningKeyPurpose]gomatrixserverlib.CrossSigningKey, error)
	CrossSigningKeysDataForUser(ctx context.Context, userID string) (types.CrossSigningKeyMap, error)
	CrossSigningSigsForTarget(ctx context.Context, originUserID, targetUserID string, targetKeyID gomatrixserverlib.KeyID) (types.CrossSigningSigMap, error)

	StoreCrossSigningKeysForUser(ctx context.Context, userID string, keyMap types.CrossSigningKeyMap) error
	StoreCrossSigningSigsForTarget(ctx context.Context, originUserID string, originKeyID gomatrixserverlib.KeyID, targetUserID string, targetKeyID gomatrixserverlib.KeyID, signature gomatrixserverlib.Base64Bytes) error
}
