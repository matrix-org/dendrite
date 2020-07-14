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

package internal

import (
	"bytes"
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/tidwall/gjson"
)

type KeyInternalAPI struct {
	db storage.Database
}

func (a *KeyInternalAPI) PerformUploadKeys(ctx context.Context, req *api.PerformUploadKeysRequest, res *api.PerformUploadKeysResponse) {
	res.KeyErrors = make(map[string]map[string]*api.KeyError)
	a.uploadDeviceKeys(ctx, req, res)
	a.uploadOneTimeKeys(ctx, req, res)
}
func (a *KeyInternalAPI) PerformClaimKeys(ctx context.Context, req *api.PerformClaimKeysRequest, res *api.PerformClaimKeysResponse) {

}
func (a *KeyInternalAPI) QueryKeys(ctx context.Context, req *api.QueryKeysRequest, res *api.QueryKeysResponse) {

}

func (a *KeyInternalAPI) uploadDeviceKeys(ctx context.Context, req *api.PerformUploadKeysRequest, res *api.PerformUploadKeysResponse) {
	var keysToStore []api.DeviceKeys
	// assert that the user ID / device ID are not lying for each key
	for _, key := range req.DeviceKeys {
		gotUserID := gjson.GetBytes(key.KeyJSON, "user_id").Str
		gotDeviceID := gjson.GetBytes(key.KeyJSON, "device_id").Str
		if gotUserID == key.UserID && gotDeviceID == key.DeviceID {
			keysToStore = append(keysToStore, key)
			continue
		}

		res.KeyError(key.UserID, key.DeviceID, &api.KeyError{
			Error: fmt.Sprintf(
				"user_id or device_id mismatch: users: %s - %s, devices: %s - %s",
				gotUserID, key.UserID, gotDeviceID, key.DeviceID,
			),
		})
	}
	// get existing device keys so we can check for changes
	existingKeys := make([]api.DeviceKeys, len(keysToStore))
	for i := range keysToStore {
		existingKeys[i] = api.DeviceKeys{
			UserID:   keysToStore[i].UserID,
			DeviceID: keysToStore[i].DeviceID,
		}
	}
	if err := a.db.DeviceKeysJSON(ctx, existingKeys); err != nil {
		res.Error = &api.KeyError{
			Error: fmt.Sprintf("failed to query existing device keys: %s", err.Error()),
		}
		return
	}
	// store the device keys and emit changes
	if err := a.db.StoreDeviceKeys(ctx, keysToStore); err != nil {
		res.Error = &api.KeyError{
			Error: fmt.Sprintf("failed to store device keys: %s", err.Error()),
		}
		return
	}
	a.emitDeviceKeyChanges(existingKeys, keysToStore)
}

func (a *KeyInternalAPI) uploadOneTimeKeys(ctx context.Context, req *api.PerformUploadKeysRequest, res *api.PerformUploadKeysResponse) {
	for _, key := range req.OneTimeKeys {
		// grab existing keys based on (user/device/algorithm/key ID)
		keyIDsWithAlgorithms := make([]string, len(key.KeyJSON))
		i := 0
		for keyIDWithAlgo := range key.KeyJSON {
			keyIDsWithAlgorithms[i] = keyIDWithAlgo
			i++
		}
		existingKeys, err := a.db.ExistingOneTimeKeys(ctx, key.UserID, key.DeviceID, keyIDsWithAlgorithms)
		if err != nil {
			res.KeyError(key.UserID, key.DeviceID, &api.KeyError{
				Error: "failed to query existing one-time keys: " + err.Error(),
			})
			continue
		}
		for keyIDWithAlgo := range existingKeys {
			// if keys exist and the JSON doesn't match, error out as the key already exists
			if !bytes.Equal(existingKeys[keyIDWithAlgo], key.KeyJSON[keyIDWithAlgo]) {
				res.KeyError(key.UserID, key.DeviceID, &api.KeyError{
					Error: fmt.Sprintf("%s device %s: algorithm / key ID %s one-time key already exists", key.UserID, key.DeviceID, keyIDWithAlgo),
				})
				continue
			}
		}
		// store one-time keys
		if err := a.db.StoreOneTimeKeys(ctx, key); err != nil {
			res.KeyError(key.UserID, key.DeviceID, &api.KeyError{
				Error: fmt.Sprintf("%s device %s : failed to store one-time keys: %s", key.UserID, key.DeviceID, err.Error()),
			})
		}
	}

}

func (a *KeyInternalAPI) emitDeviceKeyChanges(existing, new []api.DeviceKeys) {
	// TODO
}
