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
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type KeyInternalAPI struct {
	DB         storage.Database
	ThisServer gomatrixserverlib.ServerName
	FedClient  *gomatrixserverlib.FederationClient
	UserAPI    userapi.UserInternalAPI
	Producer   *producers.KeyChange
	Updater    *DeviceListUpdater
}

func (a *KeyInternalAPI) SetUserAPI(i userapi.UserInternalAPI) {
	a.UserAPI = i
}

func (a *KeyInternalAPI) InputDeviceListUpdate(
	ctx context.Context, req *api.InputDeviceListUpdateRequest, res *api.InputDeviceListUpdateResponse,
) {
	err := a.Updater.Update(ctx, req.Event)
	if err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("failed to update device list: %s", err),
		}
	}
}

func (a *KeyInternalAPI) QueryKeyChanges(ctx context.Context, req *api.QueryKeyChangesRequest, res *api.QueryKeyChangesResponse) {
	if req.Partition < 0 {
		req.Partition = a.Producer.DefaultPartition()
	}
	userIDs, latest, err := a.DB.KeyChanges(ctx, req.Partition, req.Offset, req.ToOffset)
	if err != nil {
		res.Error = &api.KeyError{
			Err: err.Error(),
		}
	}
	res.Offset = latest
	res.Partition = req.Partition
	res.UserIDs = userIDs
}

func (a *KeyInternalAPI) PerformUploadKeys(ctx context.Context, req *api.PerformUploadKeysRequest, res *api.PerformUploadKeysResponse) {
	res.KeyErrors = make(map[string]map[string]*api.KeyError)
	a.uploadLocalDeviceKeys(ctx, req, res)
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

func (a *KeyInternalAPI) QueryOneTimeKeys(ctx context.Context, req *api.QueryOneTimeKeysRequest, res *api.QueryOneTimeKeysResponse) {
	count, err := a.DB.OneTimeKeysCount(ctx, req.UserID, req.DeviceID)
	if err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("Failed to query OTK counts: %s", err),
		}
		return
	}
	res.Count = *count
}

func (a *KeyInternalAPI) QueryDeviceMessages(ctx context.Context, req *api.QueryDeviceMessagesRequest, res *api.QueryDeviceMessagesResponse) {
	msgs, err := a.DB.DeviceKeysForUser(ctx, req.UserID, nil)
	if err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("failed to query DB for device keys: %s", err),
		}
		return
	}
	maxStreamID := 0
	for _, m := range msgs {
		if m.StreamID > maxStreamID {
			maxStreamID = m.StreamID
		}
	}
	// remove deleted devices
	var result []api.DeviceMessage
	for _, m := range msgs {
		if m.KeyJSON == nil {
			continue
		}
		result = append(result, m)
	}
	res.Devices = result
	res.StreamID = maxStreamID
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
				if len(dk.KeyJSON) == 0 {
					continue // don't include blank keys
				}
				// inject display name if known (either locally or remotely)
				displayName := dk.DisplayName
				if queryRes.DeviceInfo[dk.DeviceID].DisplayName != "" {
					displayName = queryRes.DeviceInfo[dk.DeviceID].DisplayName
				}
				dk.KeyJSON, _ = sjson.SetBytes(dk.KeyJSON, "unsigned", struct {
					DisplayName string `json:"device_display_name,omitempty"`
				}{displayName})
				res.DeviceKeys[userID][dk.DeviceID] = dk.KeyJSON
			}
		} else {
			domainToDeviceKeys[domain] = make(map[string][]string)
			domainToDeviceKeys[domain][userID] = append(domainToDeviceKeys[domain][userID], deviceIDs...)
		}
	}

	// attempt to satisfy key queries from the local database first as we should get device updates pushed to us
	domainToDeviceKeys = a.remoteKeysFromDatabase(ctx, res, domainToDeviceKeys)
	if len(domainToDeviceKeys) == 0 {
		return // nothing to query
	}

	// perform key queries for remote devices
	a.queryRemoteKeys(ctx, req.Timeout, res, domainToDeviceKeys)
}

func (a *KeyInternalAPI) remoteKeysFromDatabase(
	ctx context.Context, res *api.QueryKeysResponse, domainToDeviceKeys map[string]map[string][]string,
) map[string]map[string][]string {
	fetchRemote := make(map[string]map[string][]string)
	for domain, userToDeviceMap := range domainToDeviceKeys {
		for userID, deviceIDs := range userToDeviceMap {
			// we can't safely return keys from the db when all devices are requested as we don't
			// know if one has just been added.
			if len(deviceIDs) > 0 {
				err := a.populateResponseWithDeviceKeysFromDatabase(ctx, res, userID, deviceIDs)
				if err == nil {
					continue
				}
				util.GetLogger(ctx).WithError(err).Error("populateResponseWithDeviceKeysFromDatabase")
			}
			// fetch device lists from remote
			if _, ok := fetchRemote[domain]; !ok {
				fetchRemote[domain] = make(map[string][]string)
			}
			fetchRemote[domain][userID] = append(fetchRemote[domain][userID], deviceIDs...)

		}
	}
	return fetchRemote
}

func (a *KeyInternalAPI) queryRemoteKeys(
	ctx context.Context, timeout time.Duration, res *api.QueryKeysResponse, domainToDeviceKeys map[string]map[string][]string,
) {
	resultCh := make(chan *gomatrixserverlib.RespQueryKeys, len(domainToDeviceKeys))
	// allows us to wait until all federation servers have been poked
	var wg sync.WaitGroup
	wg.Add(len(domainToDeviceKeys))
	// mutex for writing directly to res (e.g failures)
	var respMu sync.Mutex

	// fan out
	for domain, deviceKeys := range domainToDeviceKeys {
		go a.queryRemoteKeysOnServer(ctx, domain, deviceKeys, &wg, &respMu, timeout, resultCh, res)
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

func (a *KeyInternalAPI) queryRemoteKeysOnServer(
	ctx context.Context, serverName string, devKeys map[string][]string, wg *sync.WaitGroup,
	respMu *sync.Mutex, timeout time.Duration, resultCh chan<- *gomatrixserverlib.RespQueryKeys,
	res *api.QueryKeysResponse,
) {
	defer wg.Done()
	fedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// for users who we do not have any knowledge about, try to start doing device list updates for them
	// by hitting /users/devices - otherwise fallback to /keys/query which has nicer bulk properties but
	// lack a stream ID.
	var userIDsForAllDevices []string
	for userID, deviceIDs := range devKeys {
		if len(deviceIDs) == 0 {
			userIDsForAllDevices = append(userIDsForAllDevices, userID)
			delete(devKeys, userID)
		}
	}
	for _, userID := range userIDsForAllDevices {
		err := a.Updater.ManualUpdate(context.Background(), gomatrixserverlib.ServerName(serverName), userID)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"user_id":       userID,
				"server":        serverName,
			}).Error("Failed to manually update device lists for user")
			// try to do it via /keys/query
			devKeys[userID] = []string{}
			continue
		}
		// refresh entries from DB: unlike remoteKeysFromDatabase we know we previously had no device info for this
		// user so the fact that we're populating all devices here isn't a problem so long as we have devices.
		respMu.Lock()
		err = a.populateResponseWithDeviceKeysFromDatabase(ctx, res, userID, nil)
		respMu.Unlock()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"user_id":       userID,
				"server":        serverName,
			}).Error("Failed to manually update device lists for user")
			// try to do it via /keys/query
			devKeys[userID] = []string{}
			continue
		}
	}
	if len(devKeys) == 0 {
		return
	}
	queryKeysResp, err := a.FedClient.QueryKeys(fedCtx, gomatrixserverlib.ServerName(serverName), devKeys)
	if err == nil {
		resultCh <- &queryKeysResp
		return
	}
	respMu.Lock()
	res.Failures[serverName] = map[string]interface{}{
		"message": err.Error(),
	}

	// last ditch, use the cache only. This is good for when clients hit /keys/query and the remote server
	// is down, better to return something than nothing at all. Clients can know about the failure by
	// inspecting the failures map though so they can know it's a cached response.
	for userID, dkeys := range devKeys {
		// drop the error as it's already a failure at this point
		_ = a.populateResponseWithDeviceKeysFromDatabase(ctx, res, userID, dkeys)
	}
	respMu.Unlock()

}

func (a *KeyInternalAPI) populateResponseWithDeviceKeysFromDatabase(
	ctx context.Context, res *api.QueryKeysResponse, userID string, deviceIDs []string,
) error {
	keys, err := a.DB.DeviceKeysForUser(ctx, userID, deviceIDs)
	// if we can't query the db or there are fewer keys than requested, fetch from remote.
	if err != nil {
		return fmt.Errorf("DeviceKeysForUser %s %v failed: %w", userID, deviceIDs, err)
	}
	if len(keys) < len(deviceIDs) {
		return fmt.Errorf("DeviceKeysForUser %s returned fewer devices than requested, falling back to remote", userID)
	}
	if len(deviceIDs) == 0 && len(keys) == 0 {
		return fmt.Errorf("DeviceKeysForUser %s returned no keys but wanted all keys, falling back to remote", userID)
	}
	if res.DeviceKeys[userID] == nil {
		res.DeviceKeys[userID] = make(map[string]json.RawMessage)
	}

	for _, key := range keys {
		if len(key.KeyJSON) == 0 {
			continue // ignore deleted keys
		}
		// inject the display name
		key.KeyJSON, _ = sjson.SetBytes(key.KeyJSON, "unsigned", struct {
			DisplayName string `json:"device_display_name,omitempty"`
		}{key.DisplayName})
		res.DeviceKeys[userID][key.DeviceID] = key.KeyJSON
	}
	return nil
}

func (a *KeyInternalAPI) uploadLocalDeviceKeys(ctx context.Context, req *api.PerformUploadKeysRequest, res *api.PerformUploadKeysResponse) {
	var keysToStore []api.DeviceMessage
	// assert that the user ID / device ID are not lying for each key
	for _, key := range req.DeviceKeys {
		_, serverName, err := gomatrixserverlib.SplitID('@', key.UserID)
		if err != nil {
			continue // ignore invalid users
		}
		if serverName != a.ThisServer {
			continue // ignore remote users
		}
		if len(key.KeyJSON) == 0 {
			keysToStore = append(keysToStore, key.WithStreamID(0))
			continue // deleted keys don't need sanity checking
		}
		gotUserID := gjson.GetBytes(key.KeyJSON, "user_id").Str
		gotDeviceID := gjson.GetBytes(key.KeyJSON, "device_id").Str
		if gotUserID == key.UserID && gotDeviceID == key.DeviceID {
			keysToStore = append(keysToStore, key.WithStreamID(0))
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
	existingKeys := make([]api.DeviceMessage, len(keysToStore))
	for i := range keysToStore {
		existingKeys[i] = api.DeviceMessage{
			DeviceKeys: api.DeviceKeys{
				UserID:   keysToStore[i].UserID,
				DeviceID: keysToStore[i].DeviceID,
			},
		}
	}
	if err := a.DB.DeviceKeysJSON(ctx, existingKeys); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("failed to query existing device keys: %s", err.Error()),
		}
		return
	}
	if req.OnlyDisplayNameUpdates {
		// add the display name field from keysToStore into existingKeys
		keysToStore = appendDisplayNames(existingKeys, keysToStore)
	}
	// store the device keys and emit changes
	err := a.DB.StoreLocalDeviceKeys(ctx, keysToStore)
	if err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("failed to store device keys: %s", err.Error()),
		}
		return
	}
	err = a.emitDeviceKeyChanges(existingKeys, keysToStore)
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

func (a *KeyInternalAPI) emitDeviceKeyChanges(existing, new []api.DeviceMessage) error {
	// find keys in new that are not in existing
	var keysAdded []api.DeviceMessage
	for _, newKey := range new {
		exists := false
		for _, existingKey := range existing {
			// Do not treat the absence of keys as equal, or else we will not emit key changes
			// when users delete devices which never had a key to begin with as both KeyJSONs are nil.
			if bytes.Equal(existingKey.KeyJSON, newKey.KeyJSON) && len(existingKey.KeyJSON) > 0 {
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

func appendDisplayNames(existing, new []api.DeviceMessage) []api.DeviceMessage {
	for i, existingDevice := range existing {
		for _, newDevice := range new {
			if existingDevice.DeviceID != newDevice.DeviceID {
				continue
			}
			existingDevice.DisplayName = newDevice.DisplayName
			existing[i] = existingDevice
		}
	}
	return existing
}
