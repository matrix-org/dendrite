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
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

func LeaveRoomByID(
	req *http.Request,
	device *api.Device,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	roomID string,
) util.JSONResponse {
	// Prepare to ask the roomserver to perform the room join.
	leaveReq := roomserverAPI.PerformLeaveRequest{
		RoomID: roomID,
		UserID: device.UserID,
	}
	leaveRes := roomserverAPI.PerformLeaveResponse{}

	// Ask the roomserver to perform the leave.
	if err := rsAPI.PerformLeave(req.Context(), &leaveReq, &leaveRes); err != nil {
		if leaveRes.Code != 0 {
			return util.JSONResponse{
				Code: leaveRes.Code,
				JSON: jsonerror.LeaveServerNoticeError(),
			}
		}
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
