// Copyright 2020 David Spenler
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
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	var queryRes api.QueryDevicesResponse
	err := userAPI.QueryDevices(req.Context(), &api.QueryDevicesRequest{
		UserID: userID,
	}, &queryRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("GetAdminWhois failed to query user devices")
		return jsonerror.InternalServerError()
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
