// Copyright 2017 Paul Tötterman <paul.totterman@iki.fi>
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
	"io/ioutil"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/userapi/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// https://matrix.org/docs/spec/client_server/r0.6.1#get-matrix-client-r0-devices
type deviceJSON struct {
	DeviceID    string `json:"device_id"`
	DisplayName string `json:"display_name"`
	LastSeenIP  string `json:"last_seen_ip"`
	LastSeenTS  uint64 `json:"last_seen_ts"`
}

type devicesJSON struct {
	Devices []deviceJSON `json:"devices"`
}

type deviceUpdateJSON struct {
	DisplayName *string `json:"display_name"`
}

type devicesDeleteJSON struct {
	Devices []string `json:"devices"`
}

// GetDeviceByID handles /devices/{deviceID}
func GetDeviceByID(
	req *http.Request, userAPI userapi.UserInternalAPI, device *api.Device,
	deviceID string,
) util.JSONResponse {
	var queryRes userapi.QueryDevicesResponse
	err := userAPI.QueryDevices(req.Context(), &userapi.QueryDevicesRequest{
		UserID: device.UserID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryDevices failed")
		return jsonerror.InternalServerError()
	}
	var targetDevice *userapi.Device
	for _, device := range queryRes.Devices {
		if device.ID == deviceID {
			targetDevice = &device
			break
		}
	}
	if targetDevice == nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Unknown device"),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: deviceJSON{
			DeviceID:    targetDevice.ID,
			DisplayName: targetDevice.DisplayName,
		},
	}
}

// GetDevicesByLocalpart handles /devices
func GetDevicesByLocalpart(
	req *http.Request, userAPI userapi.UserInternalAPI, device *api.Device,
) util.JSONResponse {
	var queryRes userapi.QueryDevicesResponse
	err := userAPI.QueryDevices(req.Context(), &userapi.QueryDevicesRequest{
		UserID: device.UserID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryDevices failed")
		return jsonerror.InternalServerError()
	}

	res := devicesJSON{}

	for _, dev := range queryRes.Devices {
		res.Devices = append(res.Devices, deviceJSON{
			DeviceID:    dev.ID,
			DisplayName: dev.DisplayName,
		})
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

// UpdateDeviceByID handles PUT on /devices/{deviceID}
func UpdateDeviceByID(
	req *http.Request, userAPI api.UserInternalAPI, device *api.Device,
	deviceID string,
) util.JSONResponse {

	defer req.Body.Close() // nolint: errcheck

	payload := deviceUpdateJSON{}

	if resErr := httputil.UnmarshalJSONRequest(req, &payload); resErr != nil {
		return *resErr
	}

	var performRes api.PerformDeviceUpdateResponse
	err := userAPI.PerformDeviceUpdate(req.Context(), &api.PerformDeviceUpdateRequest{
		RequestingUserID: device.UserID,
		DeviceID:         deviceID,
		DisplayName:      payload.DisplayName,
	}, &performRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("PerformDeviceUpdate failed")
		return jsonerror.InternalServerError()
	}
	if !performRes.DeviceExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.Forbidden("device does not exist"),
		}
	}
	if performRes.Forbidden {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("device not owned by current user"),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteDeviceById handles DELETE requests to /devices/{deviceId}
func DeleteDeviceById(
	req *http.Request, userInteractiveAuth *auth.UserInteractive, userAPI api.UserInternalAPI, device *api.Device,
	deviceID string,
) util.JSONResponse {
	ctx := req.Context()
	defer req.Body.Close() // nolint:errcheck
	bodyBytes, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be read: " + err.Error()),
		}
	}
	login, errRes := userInteractiveAuth.Verify(ctx, bodyBytes, device)
	if errRes != nil {
		return *errRes
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	// make sure that the access token being used matches the login creds used for user interactive auth, else
	// 1 compromised access token could be used to logout all devices.
	if login.Username() != localpart && login.Username() != device.UserID {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("Cannot delete another user's device"),
		}
	}

	var res api.PerformDeviceDeletionResponse
	if err := userAPI.PerformDeviceDeletion(ctx, &api.PerformDeviceDeletionRequest{
		UserID:    device.UserID,
		DeviceIDs: []string{deviceID},
	}, &res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("userAPI.PerformDeviceDeletion failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteDevices handles POST requests to /delete_devices
func DeleteDevices(
	req *http.Request, userAPI api.UserInternalAPI, device *api.Device,
) util.JSONResponse {
	ctx := req.Context()
	payload := devicesDeleteJSON{}

	if resErr := httputil.UnmarshalJSONRequest(req, &payload); resErr != nil {
		return *resErr
	}

	defer req.Body.Close() // nolint: errcheck

	var res api.PerformDeviceDeletionResponse
	if err := userAPI.PerformDeviceDeletion(ctx, &api.PerformDeviceDeletionRequest{
		UserID:    device.UserID,
		DeviceIDs: payload.Devices,
	}, &res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("userAPI.PerformDeviceDeletion failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
