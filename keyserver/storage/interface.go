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
)

type Database interface {
	// ExistingOneTimeKeys returns a map of keyIDWithAlgorithm to key JSON for the given parameters. If no keys exist with this combination
	// of user/device/key/algorithm 4-uple then it is omitted from the map. Returns an error when failing to communicate with the database.
	ExistingOneTimeKeys(ctx context.Context, userID, deviceID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error)

	// StoreOneTimeKeys persists the given one-time keys.
	StoreOneTimeKeys(ctx context.Context, keys api.OneTimeKeys) (*api.OneTimeKeysCount, error)

	// DeviceKeysJSON populates the KeyJSON for the given keys. If any proided `keys` have a `KeyJSON` already then it will be replaced.
	DeviceKeysJSON(ctx context.Context, keys []api.DeviceKeys) error

	// StoreDeviceKeys persists the given keys. Keys with the same user ID and device ID will be replaced.
	// Returns an error if there was a problem storing the keys.
	StoreDeviceKeys(ctx context.Context, keys []api.DeviceKeys) error

	// DeviceKeysForUser returns the device keys for the device IDs given. If the length of deviceIDs is 0, all devices are selected.
	// If there are some missing keys, they are omitted from the returned slice. There is no ordering on the returned slice.
	DeviceKeysForUser(ctx context.Context, userID string, deviceIDs []string) ([]api.DeviceKeys, error)
}
