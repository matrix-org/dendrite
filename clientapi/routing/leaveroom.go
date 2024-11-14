// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

func LeaveRoomByID(
	req *http.Request,
	device *api.Device,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	roomID string,
) util.JSONResponse {
	userID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("device userID is invalid"),
		}
	}

	// Prepare to ask the roomserver to perform the room join.
	leaveReq := roomserverAPI.PerformLeaveRequest{
		RoomID: roomID,
		Leaver: *userID,
	}
	leaveRes := roomserverAPI.PerformLeaveResponse{}

	// Ask the roomserver to perform the leave.
	if err := rsAPI.PerformLeave(req.Context(), &leaveReq, &leaveRes); err != nil {
		if leaveRes.Code != 0 {
			return util.JSONResponse{
				Code: leaveRes.Code,
				JSON: spec.LeaveServerNoticeError(),
			}
		}
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
