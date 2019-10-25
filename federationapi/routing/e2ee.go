// Copyright Sumukha PK 2019
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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/encryptoapi/storage"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// ClaimKeys provides the e2ee keys of the user
func ClaimKeys(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	encryptionDB *storage.Database,
) util.JSONResponse {

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// QueryKeys provides the public identity keys and supported algorithms.
func QueryKeys(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	encryptionDB *storage.Database,
) util.JSONResponse {
	var err error
	var queryRq types.QueryRequest
	if reqErr := httputil.UnmarshalJSONRequest(httpReq, &queryRq); reqErr != nil {
		return *reqErr
	}
	queryRp := types.QueryResponse{}

	queryRp.DeviceKeys = make(map[string]map[string]types.DeviceKeys)
	// query one's device key from user corresponding to uid
	for uid, arr := range queryRq.DeviceKeys {
		queryRp.DeviceKeys[uid] = make(map[string]types.DeviceKeys)
		deviceKeysQueryMap := queryRp.DeviceKeys[uid]
		// backward compatible to old interface
		midArr := []string{}
		// figure out device list from devices described as device which is actually deviceID
		for device := range arr.(map[string]interface{}) {
			midArr = append(midArr, device)
		}
		// all device keys
		dkeys, _ := encryptionDB.QueryInRange(httpReq.Context(), uid, midArr)
		// build response for them

		for _, key := range dkeys {

			deviceKeysQueryMap = presetDeviceKeysQueryMap(deviceKeysQueryMap, uid, key)
			// load for accomplishment
			single := deviceKeysQueryMap[key.DeviceID]
			resKey := fmt.Sprintf("%s:%s", key.KeyAlgorithm, key.DeviceID)
			resBody := key.Key
			single.Keys[resKey] = resBody
			single.DeviceID = key.DeviceID
			single.UserID = key.UserID
			single.Signature[uid][fmt.Sprintf("%s:%s", "ed25519", key.DeviceID)] = key.Signature
			single.Algorithm, err = takeAL(httpReq.Context(), *encryptionDB, key.UserID, key.DeviceID)
			localpart, _, _ := gomatrixserverlib.SplitID('@', uid)
			device, _ := deviceDB.GetDeviceByID(req.Context(), localpart, deviceID)
			single.Unsigned.Info = device.DisplayName
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
		JSON: struct{}{},
	}
}

func presetDeviceKeysQueryMap(
	deviceKeysQueryMap map[string]types.DeviceKeys,
	uid string,
	key types.KeyHolder,
) map[string]types.DeviceKeys {
	// preset for complicated nested map struct
	if _, ok := deviceKeysQueryMap[key.DeviceID]; !ok {
		// make consistency
		deviceKeysQueryMap[key.DeviceID] = types.DeviceKeysQuery{}
	}
	if deviceKeysQueryMap[key.DeviceID].Signature == nil {
		mid := make(map[string]map[string]string)
		midmap := deviceKeysQueryMap[key.DeviceID]
		midmap.Signature = mid
		deviceKeysQueryMap[key.DeviceID] = midmap
	}
	if deviceKeysQueryMap[key.DeviceID].Keys == nil {
		mid := make(map[string]string)
		midmap := deviceKeysQueryMap[key.DeviceID]
		midmap.Keys = mid
		deviceKeysQueryMap[key.DeviceID] = midmap
	}
	if _, ok := deviceKeysQueryMap[key.DeviceID].Signature[uid]; !ok {
		// make consistency
		deviceKeysQueryMap[key.DeviceID].Signature[uid] = make(map[string]string)
	}
	return deviceKeysQueryMap
}

func takeAL(
	ctx context.Context,
	encryptDB storage.Database,
	uid, device string,
) (al []string, err error) {
	al, err = encryptDB.SelectAl(ctx, uid, device)
	return
}
