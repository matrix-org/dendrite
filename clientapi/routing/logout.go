// Copyright 2017 Vector Creations Ltd
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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// Logout handles POST /logout
func Logout(
	req *http.Request, userAPI api.ClientUserAPI, device *api.Device,
) util.JSONResponse {
	var performRes api.PerformDeviceDeletionResponse
	err := userAPI.PerformDeviceDeletion(req.Context(), &api.PerformDeviceDeletionRequest{
		UserID:    device.UserID,
		DeviceIDs: []string{device.ID},
	}, &performRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("PerformDeviceDeletion failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// LogoutAll handles POST /logout/all
func LogoutAll(
	req *http.Request, userAPI api.ClientUserAPI, device *api.Device,
) util.JSONResponse {
	var performRes api.PerformDeviceDeletionResponse
	err := userAPI.PerformDeviceDeletion(req.Context(), &api.PerformDeviceDeletionRequest{
		UserID:    device.UserID,
		DeviceIDs: nil,
	}, &performRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("PerformDeviceDeletion failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
