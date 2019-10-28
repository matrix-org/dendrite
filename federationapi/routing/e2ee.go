// Copyright 2019 Sumukha PK
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

package routing

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	ecTypes "github.com/matrix-org/dendrite/encryptoapi/types"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

const (
	// ONETIMEKEYSTRING key string
	ONETIMEKEYSTRING = iota
	// ONETIMEKEYOBJECT key object
	ONETIMEKEYOBJECT
)

// ONETIMEKEYSTR stands for storage string property
const ONETIMEKEYSTR = "one_time_key"

// DEVICEKEYSTR stands for storage string property
const DEVICEKEYSTR = "device_key"

// ClaimKeys provides the e2ee keys of the user
func ClaimKeys(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	encryptionDB *storage.Database,
) util.JSONResponse {
	var claimReq types.ClaimRequest
	claimRes := types.ClaimResponse{}
	claimRes.OneTimeKeys = make(map[string]map[string]map[string]interface{})
	if reqErr := httputil.UnmarshalJSONRequest(httpReq, &claimReq); reqErr != nil {
		return *reqErr
	}

	content := claimReq.OneTimeKeys
	for uid, detail := range content {
		for deviceID, alg := range detail {
			var algTyp int
			if strings.Contains(alg, "signed") {
				algTyp = ONETIMEKEYOBJECT
			} else {
				algTyp = ONETIMEKEYSTRING
			}
			key, err := pickOne(httpReq.Context(), *encryptionDB, uid, deviceID, alg)
			if err != nil {
				// send a better response in order to capture failures on the other part
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: struct{}{},
				}
				// claimRes.Failures[uid] = fmt.Sprintf("%s: %s", "failed to get keys for device", deviceID)
			}
			claimRes.OneTimeKeys[uid] = make(map[string]map[string]interface{})
			keyPreMap := claimRes.OneTimeKeys[uid]
			keymap := keyPreMap[deviceID]
			if keymap == nil {
				keymap = make(map[string]interface{})
			}
			switch algTyp {
			case ONETIMEKEYSTRING:
				keymap[fmt.Sprintf("%s:%s", alg, key.KeyID)] = key.Key
			case ONETIMEKEYOBJECT:
				sig := make(map[string]map[string]string)
				sig[uid] = make(map[string]string)
				sig[uid][fmt.Sprintf("%s:%s", "ed25519", deviceID)] = key.Signature
				keymap[fmt.Sprintf("%s:%s", alg, key.KeyID)] = types.KeyObject{Key: key.Key, Signatures: sig}
			}
			claimRes.OneTimeKeys[uid][deviceID] = keymap
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: claimRes,
	}
}

// QueryKeys provides the public identity keys and supported algorithms.
func QueryKeys(
	httpReq *http.Request,
	encryptionDB *storage.Database,
	deviceID string,
	deviceDB *devices.Database,
) util.JSONResponse {
	var err error
	var queryReq types.QueryRequest
	if reqErr := httputil.UnmarshalJSONRequest(httpReq, &queryReq); reqErr != nil {
		return *reqErr
	}
	queryRes := types.QueryResponse{}

	queryRes.DeviceKeys = make(map[string]map[string]types.DeviceKeys)
	// iterate through all demanded device keys
	for uid, arr := range queryReq.DeviceKeys {
		queryRes.DeviceKeys[uid] = make(map[string]types.DeviceKeys)
		deviceKeysQueryMap := queryRes.DeviceKeys[uid]
		// backward compatible to old interface
		// midArr := []string{}
		// figure out device list from devices described as device which is actually deviceID
		// for device := range arr.(map[string]interface{}) {
		// 	midArr = append(midArr, device)
		// }
		// all device keys
		dkeys, err := encryptionDB.QueryInRange(httpReq.Context(), uid, arr)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: struct{}{},
			}
		}
		// build response for them
		for _, key := range dkeys {
			deviceKeysQueryMap = presetDeviceKeysQueryMap(deviceKeysQueryMap, uid, key)
			// group the keys as intended in the response
			single := deviceKeysQueryMap[key.DeviceID]
			resKey := fmt.Sprintf("%s:%s", key.KeyAlgorithm, key.DeviceID)
			resBody := key.Key
			single.Keys[resKey] = resBody
			single.DeviceID = key.DeviceID
			single.UserID = key.UserID
			single.Signatures[uid][fmt.Sprintf("%s:%s", "ed25519", key.DeviceID)] = key.Signature
			single.Algorithms, err = takeAlgo(httpReq.Context(), *encryptionDB, key.UserID, key.DeviceID)
			localpart, _, _ := gomatrixserverlib.SplitID('@', uid)
			device, _ := deviceDB.GetDeviceByID(httpReq.Context(), localpart, deviceID)
			single.Unsigned.DeviceDisplayName = device.DisplayName
			deviceKeysQueryMap[key.DeviceID] = single
		}
	}
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: struct{}{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRes,
	}
}

func presetDeviceKeysQueryMap(
	deviceKeysQueryMap map[string]types.DeviceKeys,
	uid string,
	key ecTypes.KeyHolder,
) map[string]types.DeviceKeys {
	// preset for complicated nested map struct
	if _, ok := deviceKeysQueryMap[key.DeviceID]; !ok {
		// make consistency
		deviceKeysQueryMap[key.DeviceID] = types.DeviceKeys{}
	}
	if deviceKeysQueryMap[key.DeviceID].Signatures == nil {
		mid := make(map[string]map[string]string)
		midmap := deviceKeysQueryMap[key.DeviceID]
		midmap.Signatures = mid
		deviceKeysQueryMap[key.DeviceID] = midmap
	}
	if deviceKeysQueryMap[key.DeviceID].Keys == nil {
		mid := make(map[string]string)
		midmap := deviceKeysQueryMap[key.DeviceID]
		midmap.Keys = mid
		deviceKeysQueryMap[key.DeviceID] = midmap
	}
	if _, ok := deviceKeysQueryMap[key.DeviceID].Signatures[uid]; !ok {
		// make consistency
		deviceKeysQueryMap[key.DeviceID].Signatures[uid] = make(map[string]string)
	}
	return deviceKeysQueryMap
}

func takeAlgo(
	ctx context.Context,
	encryptDB storage.Database,
	uid, device string,
) (al []string, err error) {
	al, err = encryptDB.SelectAlgo(ctx, uid, device)
	return
}

func pickOne(
	ctx context.Context,
	encryptDB storage.Database,
	uid, device, al string,
) (key types.KeyHolder, err error) {
	key, err = encryptDB.SelectOneTimeKeySingle(ctx, uid, device, al)
	return
}
