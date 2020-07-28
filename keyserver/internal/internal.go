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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/producers"
	"github.com/matrix-org/dendrite/keyserver/storage"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type KeyInternalAPI struct {
	DB         storage.Database
	ThisServer gomatrixserverlib.ServerName
	FedClient  *gomatrixserverlib.FederationClient
	UserAPI    userapi.UserInternalAPI
	Producer   *producers.KeyChange
}

func (a *KeyInternalAPI) QueryKeyChanges(ctx context.Context, req *api.QueryKeyChangesRequest, res *api.QueryKeyChangesResponse) {
	userIDs, latest, err := a.DB.KeyChanges(ctx, req.Partition, req.Offset)
	if err != nil {
		res.Error = &api.KeyError{
			Err: err.Error(),
		}
	}
	res.Offset = latest
	res.UserIDs = userIDs
}

func (a *KeyInternalAPI) PerformUploadKeys(ctx context.Context, req *api.PerformUploadKeysRequest, res *api.PerformUploadKeysResponse) {
	res.KeyErrors = make(map[string]map[string]*api.KeyError)
	a.uploadDeviceKeys(ctx, req, res)
	a.uploadOneTimeKeys(ctx, req, res)
}

func (a *KeyInternalAPI) PerformClaimKeys(ctx context.Context, req *api.PerformClaimKeysRequest, res *api.PerformClaimKeysResponse) {
	res.OneTimeKeys = make(map[string]map[string]map[string]json.RawMessage)
	res.Failures = make(map[string]interface{})
	// wrap request map in a top-level by-domain map
	domainToDeviceKeys := make(map[string]map[string]map[string]string)
	for userID, val := range req.OneTimeKeys {
		_, serverName, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil {
			continue // ignore invalid users
		}
		nested, ok := domainToDeviceKeys[string(serverName)]
		if !ok {
			nested = make(map[string]map[string]string)
		}
		nested[userID] = val
		domainToDeviceKeys[string(serverName)] = nested
	}
	// claim local keys
	if local, ok := domainToDeviceKeys[string(a.ThisServer)]; ok {
		keys, err := a.DB.ClaimKeys(ctx, local)
		if err != nil {
			res.Error = &api.KeyError{
				Err: fmt.Sprintf("failed to ClaimKeys locally: %s", err),
			}
		}
		util.GetLogger(ctx).WithField("keys_claimed", len(keys)).WithField("num_users", len(local)).Info("Claimed local keys")
		for _, key := range keys {
			_, ok := res.OneTimeKeys[key.UserID]
			if !ok {
				res.OneTimeKeys[key.UserID] = make(map[string]map[string]json.RawMessage)
			}
			_, ok = res.OneTimeKeys[key.UserID][key.DeviceID]
			if !ok {
				res.OneTimeKeys[key.UserID][key.DeviceID] = make(map[string]json.RawMessage)
			}
			for keyID, keyJSON := range key.KeyJSON {
				res.OneTimeKeys[key.UserID][key.DeviceID][keyID] = keyJSON
			}
		}
		delete(domainToDeviceKeys, string(a.ThisServer))
	}
	if len(domainToDeviceKeys) > 0 {
		a.claimRemoteKeys(ctx, req.Timeout, res, domainToDeviceKeys)
	}
}

func (a *KeyInternalAPI) claimRemoteKeys(
	ctx context.Context, timeout time.Duration, res *api.PerformClaimKeysResponse, domainToDeviceKeys map[string]map[string]map[string]string,
) {
	resultCh := make(chan *gomatrixserverlib.RespClaimKeys, len(domainToDeviceKeys))
	// allows us to wait until all federation servers have been poked
	var wg sync.WaitGroup
	wg.Add(len(domainToDeviceKeys))
	// mutex for failures
	var failMu sync.Mutex
	util.GetLogger(ctx).WithField("num_servers", len(domainToDeviceKeys)).Info("Claiming remote keys from servers")

	// fan out
	for d, k := range domainToDeviceKeys {
		go func(domain string, keysToClaim map[string]map[string]string) {
			defer wg.Done()
			fedCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			claimKeyRes, err := a.FedClient.ClaimKeys(fedCtx, gomatrixserverlib.ServerName(domain), keysToClaim)
			if err != nil {
				util.GetLogger(ctx).WithError(err).WithField("server", domain).Error("ClaimKeys failed")
				failMu.Lock()
				res.Failures[domain] = map[string]interface{}{
					"message": err.Error(),
				}
				failMu.Unlock()
				return
			}
			resultCh <- &claimKeyRes
		}(d, k)
	}

	// Close the result channel when the goroutines have quit so the for .. range exits
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	keysClaimed := 0
	for result := range resultCh {
		for userID, nest := range result.OneTimeKeys {
			res.OneTimeKeys[userID] = make(map[string]map[string]json.RawMessage)
			for deviceID, nest2 := range nest {
				res.OneTimeKeys[userID][deviceID] = make(map[string]json.RawMessage)
				for keyIDWithAlgo, otk := range nest2 {
					keyJSON, err := json.Marshal(otk)
					if err != nil {
						continue
					}
					res.OneTimeKeys[userID][deviceID][keyIDWithAlgo] = keyJSON
					keysClaimed++
				}
			}
		}
	}
	util.GetLogger(ctx).WithField("num_keys", keysClaimed).Info("Claimed remote keys")
}

func (a *KeyInternalAPI) QueryKeys(ctx context.Context, req *api.QueryKeysRequest, res *api.QueryKeysResponse) {
	res.DeviceKeys = make(map[string]map[string]json.RawMessage)
	res.Failures = make(map[string]interface{})
	// make a map from domain to device keys
	domainToDeviceKeys := make(map[string]map[string][]string)
	for userID, deviceIDs := range req.UserToDevices {
		_, serverName, err := gomatrixserverlib.SplitID('@', userID)
		if err != nil {
			continue // ignore invalid users
		}
		domain := string(serverName)
		// query local devices
		if serverName == a.ThisServer {
			deviceKeys, err := a.DB.DeviceKeysForUser(ctx, userID, deviceIDs)
			if err != nil {
				res.Error = &api.KeyError{
					Err: fmt.Sprintf("failed to query local device keys: %s", err),
				}
				return
			}

			// pull out display names after we have the keys so we handle wildcards correctly
			var dids []string
			for _, dk := range deviceKeys {
				dids = append(dids, dk.DeviceID)
			}
			var queryRes userapi.QueryDeviceInfosResponse
			err = a.UserAPI.QueryDeviceInfos(ctx, &userapi.QueryDeviceInfosRequest{
				DeviceIDs: dids,
			}, &queryRes)
			if err != nil {
				util.GetLogger(ctx).Warnf("Failed to QueryDeviceInfos for device IDs, display names will be missing")
			}

			if res.DeviceKeys[userID] == nil {
				res.DeviceKeys[userID] = make(map[string]json.RawMessage)
			}
			for _, dk := range deviceKeys {
				// inject display name if known
				dk.KeyJSON, _ = sjson.SetBytes(dk.KeyJSON, "unsigned", struct {
					DisplayName string `json:"device_display_name,omitempty"`
				}{queryRes.DeviceInfo[dk.DeviceID].DisplayName})
				res.DeviceKeys[userID][dk.DeviceID] = dk.KeyJSON
			}
		} else {
			domainToDeviceKeys[domain] = make(map[string][]string)
			domainToDeviceKeys[domain][userID] = append(domainToDeviceKeys[domain][userID], deviceIDs...)
		}
	}
	// TODO: set device display names when they are known

	// perform key queries for remote devices
	a.queryRemoteKeys(ctx, req.Timeout, res, domainToDeviceKeys)
}

func (a *KeyInternalAPI) queryRemoteKeys(
	ctx context.Context, timeout time.Duration, res *api.QueryKeysResponse, domainToDeviceKeys map[string]map[string][]string,
) {
	resultCh := make(chan *gomatrixserverlib.RespQueryKeys, len(domainToDeviceKeys))
	// allows us to wait until all federation servers have been poked
	var wg sync.WaitGroup
	wg.Add(len(domainToDeviceKeys))
	// mutex for failures
	var failMu sync.Mutex

	// fan out
	for domain, deviceKeys := range domainToDeviceKeys {
		go func(serverName string, devKeys map[string][]string) {
			defer wg.Done()
			fedCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			queryKeysResp, err := a.FedClient.QueryKeys(fedCtx, gomatrixserverlib.ServerName(serverName), devKeys)
			if err != nil {
				failMu.Lock()
				res.Failures[serverName] = map[string]interface{}{
					"message": err.Error(),
				}
				failMu.Unlock()
				return
			}
			resultCh <- &queryKeysResp
		}(domain, deviceKeys)
	}

	// Close the result channel when the goroutines have quit so the for .. range exits
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for result := range resultCh {
		for userID, nest := range result.DeviceKeys {
			res.DeviceKeys[userID] = make(map[string]json.RawMessage)
			for deviceID, deviceKey := range nest {
				keyJSON, err := json.Marshal(deviceKey)
				if err != nil {
					continue
				}
				res.DeviceKeys[userID][deviceID] = keyJSON
			}
		}
	}
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
			Err: fmt.Sprintf(
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
	if err := a.DB.DeviceKeysJSON(ctx, existingKeys); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("failed to query existing device keys: %s", err.Error()),
		}
		return
	}
	// store the device keys and emit changes
	if err := a.DB.StoreDeviceKeys(ctx, keysToStore); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("failed to store device keys: %s", err.Error()),
		}
		return
	}
	err := a.emitDeviceKeyChanges(existingKeys, keysToStore)
	if err != nil {
		util.GetLogger(ctx).Errorf("Failed to emitDeviceKeyChanges: %s", err)
	}
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
		existingKeys, err := a.DB.ExistingOneTimeKeys(ctx, key.UserID, key.DeviceID, keyIDsWithAlgorithms)
		if err != nil {
			res.KeyError(key.UserID, key.DeviceID, &api.KeyError{
				Err: "failed to query existing one-time keys: " + err.Error(),
			})
			continue
		}
		for keyIDWithAlgo := range existingKeys {
			// if keys exist and the JSON doesn't match, error out as the key already exists
			if !bytes.Equal(existingKeys[keyIDWithAlgo], key.KeyJSON[keyIDWithAlgo]) {
				res.KeyError(key.UserID, key.DeviceID, &api.KeyError{
					Err: fmt.Sprintf("%s device %s: algorithm / key ID %s one-time key already exists", key.UserID, key.DeviceID, keyIDWithAlgo),
				})
				continue
			}
		}
		// store one-time keys
		counts, err := a.DB.StoreOneTimeKeys(ctx, key)
		if err != nil {
			res.KeyError(key.UserID, key.DeviceID, &api.KeyError{
				Err: fmt.Sprintf("%s device %s : failed to store one-time keys: %s", key.UserID, key.DeviceID, err.Error()),
			})
			continue
		}
		// collect counts
		res.OneTimeKeyCounts = append(res.OneTimeKeyCounts, *counts)
	}

}

func (a *KeyInternalAPI) emitDeviceKeyChanges(existing, new []api.DeviceKeys) error {
	// find keys in new that are not in existing
	var keysAdded []api.DeviceKeys
	for _, newKey := range new {
		exists := false
		for _, existingKey := range existing {
			if bytes.Equal(existingKey.KeyJSON, newKey.KeyJSON) {
				exists = true
				break
			}
		}
		if !exists {
			keysAdded = append(keysAdded, newKey)
		}
	}
	return a.Producer.ProduceKeyChanges(keysAdded)
}
