// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// whoamiResponse represents an response for a `whoami` request
type whoamiResponse struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	IsGuest  bool   `json:"is_guest"`
}

// Whoami implements `/account/whoami` which enables client to query their account user id.
// https://matrix.org/docs/spec/client_server/r0.3.0.html#get-matrix-client-r0-account-whoami
func Whoami(req *http.Request, device *api.Device) util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: whoamiResponse{
			UserID:   device.UserID,
			DeviceID: device.ID,
			IsGuest:  device.AccountType == api.AccountTypeGuest,
		},
	}
}
