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

package routing

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/keyserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

func QueryKeys(
	req *http.Request,
) util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			"failures":    map[string]interface{}{},
			"device_keys": map[string]interface{}{},
		},
	}
}

type uploadKeysRequest struct {
	DeviceKeys  json.RawMessage            `json:"device_keys"`
	OneTimeKeys map[string]json.RawMessage `json:"one_time_keys"`
}

func UploadKeys(req *http.Request, keyAPI api.KeyInternalAPI, device *userapi.Device) util.JSONResponse {
	var r uploadKeysRequest
	resErr := httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	uploadReq := &api.PerformUploadKeysRequest{}
	if r.DeviceKeys != nil {
		uploadReq.DeviceKeys = []api.DeviceKeys{
			{
				DeviceID: device.ID,
				UserID:   device.UserID,
				KeyJSON:  r.DeviceKeys,
			},
		}
	}
	if r.OneTimeKeys != nil {
		uploadReq.OneTimeKeys = []api.OneTimeKeys{
			{
				DeviceID: device.ID,
				UserID:   device.UserID,
				KeyJSON:  r.OneTimeKeys,
			},
		}
	}

	var uploadRes api.PerformUploadKeysResponse
	keyAPI.PerformUploadKeys(req.Context(), uploadReq, &uploadRes)
	if uploadRes.Error != nil {
		util.GetLogger(req.Context()).WithError(uploadRes.Error).Error("Failed to PerformUploadKeys")
		return jsonerror.InternalServerError()
	}
	if len(uploadRes.KeyErrors) > 0 {
		util.GetLogger(req.Context()).WithField("key_errors", uploadRes.KeyErrors).Error("Failed to upload one or more keys")
		return util.JSONResponse{
			Code: 400,
			JSON: uploadRes.KeyErrors,
		}
	}
	return util.JSONResponse{
		Code: 200,
		JSON: struct {
			OTKCounts interface{} `json:"one_time_key_counts"`
		}{uploadRes.OneTimeKeyCounts[0].KeyCount},
	}
}
