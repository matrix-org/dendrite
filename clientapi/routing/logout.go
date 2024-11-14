// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
