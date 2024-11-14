// Copyright 2024 New Vector Ltd.
// Copyright 2020 David Spenler
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

type adminWhoisResponse struct {
	UserID  string                `json:"user_id"`
	Devices map[string]deviceInfo `json:"devices"`
}

type deviceInfo struct {
	Sessions []sessionInfo `json:"sessions"`
}

type sessionInfo struct {
	Connections []connectionInfo `json:"connections"`
}

type connectionInfo struct {
	IP        string `json:"ip"`
	LastSeen  int64  `json:"last_seen"`
	UserAgent string `json:"user_agent"`
}

// GetAdminWhois implements GET /admin/whois/{userId}
func GetAdminWhois(
	req *http.Request, userAPI api.ClientUserAPI, device *api.Device,
	userID string,
) util.JSONResponse {
	allowed := device.AccountType == api.AccountTypeAdmin || userID == device.UserID
	if !allowed {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID does not match the current user"),
		}
	}

	var queryRes api.QueryDevicesResponse
	err := userAPI.QueryDevices(req.Context(), &api.QueryDevicesRequest{
		UserID: userID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("GetAdminWhois failed to query user devices")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	devices := make(map[string]deviceInfo)
	for _, device := range queryRes.Devices {
		connInfo := connectionInfo{
			IP:        device.LastSeenIP,
			LastSeen:  device.LastSeenTS,
			UserAgent: device.UserAgent,
		}
		dev, ok := devices[device.ID]
		if !ok {
			dev.Sessions = []sessionInfo{{}}
		}
		dev.Sessions[0].Connections = append(dev.Sessions[0].Connections, connInfo)
		devices[device.ID] = dev
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: adminWhoisResponse{
			UserID:  userID,
			Devices: devices,
		},
	}
}
