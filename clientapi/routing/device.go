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
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type deviceJSON struct {
	DeviceID string `json:"device_id"`
	UserID   string `json:"user_id"`
}

type devicesJSON struct {
	Devices []deviceJSON `json:"devices"`
}

type deviceUpdateJSON struct {
	DisplayName *string `json:"display_name"`
}

// GetDeviceByID handles /devices/{deviceID}
func GetDeviceByID(
	req *http.Request, deviceDB *devices.Database, device *authtypes.Device,
	deviceID string,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	ctx := req.Context()
	dev, err := deviceDB.GetDeviceByID(ctx, localpart, deviceID)
	if err == sql.ErrNoRows {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Unknown device"),
		}
	} else if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: deviceJSON{
			DeviceID: dev.ID,
			UserID:   dev.UserID,
		},
	}
}

// GetDevicesByLocalpart handles /devices
func GetDevicesByLocalpart(
	req *http.Request, deviceDB *devices.Database, device *authtypes.Device,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	ctx := req.Context()
	deviceList, err := deviceDB.GetDevicesByLocalpart(ctx, localpart)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	res := devicesJSON{}

	for _, dev := range deviceList {
		res.Devices = append(res.Devices, deviceJSON{
			DeviceID: dev.ID,
			UserID:   dev.UserID,
		})
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}

// UpdateDeviceByID handles PUT on /devices/{deviceID}
func UpdateDeviceByID(
	req *http.Request, deviceDB *devices.Database, device *authtypes.Device,
	deviceID string,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	ctx := req.Context()
	dev, err := deviceDB.GetDeviceByID(ctx, localpart, deviceID)
	if err == sql.ErrNoRows {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Unknown device"),
		}
	} else if err != nil {
		return httputil.LogThenError(req, err)
	}

	if dev.UserID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("device not owned by current user"),
		}
	}

	defer req.Body.Close() // nolint: errcheck

	payload := deviceUpdateJSON{}

	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		return httputil.LogThenError(req, err)
	}

	if err := deviceDB.UpdateDevice(ctx, localpart, deviceID, payload.DisplayName); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteDeviceById handles DELETE requests to /devices/{deviceId}
func DeleteDeviceById(
	req *http.Request, deviceDB *devices.Database, device *authtypes.Device,
	deviceID string,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	ctx := req.Context()

	defer req.Body.Close() // nolint: errcheck

	if err := deviceDB.RemoveDevice(ctx, deviceID, localpart); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteDevices handles POST requests to /delete_devices
func DeleteDevices(
	req *http.Request, deviceDB *devices.Database, device *authtypes.Device,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	ctx := req.Context()
	d := struct {
		Devices []string `json:"devices"`
	}{}

	if err := json.NewDecoder(req.Body).Decode(&d); err != nil {
		return httputil.LogThenError(req, err)
	}

	defer req.Body.Close() // nolint: errcheck

	if err := deviceDB.RemoveDevices(ctx, localpart, d.Devices); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
