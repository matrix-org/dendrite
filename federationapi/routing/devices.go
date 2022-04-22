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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetUserDevices for the given user id
func GetUserDevices(
	req *http.Request,
	keyAPI keyapi.KeyInternalAPI,
	userID string,
) util.JSONResponse {
	var res keyapi.QueryDeviceMessagesResponse
	keyAPI.QueryDeviceMessages(req.Context(), &keyapi.QueryDeviceMessagesRequest{
		UserID: userID,
	}, &res)
	if res.Error != nil {
		util.GetLogger(req.Context()).WithError(res.Error).Error("keyAPI.QueryDeviceMessages failed")
		return jsonerror.InternalServerError()
	}

	sigReq := &keyapi.QuerySignaturesRequest{
		TargetIDs: map[string][]gomatrixserverlib.KeyID{
			userID: {},
		},
	}
	sigRes := &keyapi.QuerySignaturesResponse{}
	for _, dev := range res.Devices {
		sigReq.TargetIDs[userID] = append(sigReq.TargetIDs[userID], gomatrixserverlib.KeyID(dev.DeviceID))
	}
	keyAPI.QuerySignatures(req.Context(), sigReq, sigRes)

	response := gomatrixserverlib.RespUserDevices{
		UserID:   userID,
		StreamID: res.StreamID,
		Devices:  []gomatrixserverlib.RespUserDevice{},
	}

	if masterKey, ok := sigRes.MasterKeys[userID]; ok {
		response.MasterKey = &masterKey
	}
	if selfSigningKey, ok := sigRes.SelfSigningKeys[userID]; ok {
		response.SelfSigningKey = &selfSigningKey
	}

	for _, dev := range res.Devices {
		var key gomatrixserverlib.RespUserDeviceKeys
		err := json.Unmarshal(dev.DeviceKeys.KeyJSON, &key)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Warnf("malformed device key: %s", string(dev.DeviceKeys.KeyJSON))
			continue
		}

		device := gomatrixserverlib.RespUserDevice{
			DeviceID:    dev.DeviceID,
			DisplayName: dev.DisplayName,
			Keys:        key,
		}

		if targetUser, ok := sigRes.Signatures[userID]; ok {
			if targetKey, ok := targetUser[gomatrixserverlib.KeyID(dev.DeviceID)]; ok {
				for sourceUserID, forSourceUser := range targetKey {
					for sourceKeyID, sourceKey := range forSourceUser {
						if _, ok := device.Keys.Signatures[sourceUserID]; !ok {
							device.Keys.Signatures[sourceUserID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{}
						}
						device.Keys.Signatures[sourceUserID][sourceKeyID] = sourceKey
					}
				}
			}
		}

		response.Devices = append(response.Devices, device)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}
