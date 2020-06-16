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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type getMembershipResponse struct {
	Chunk []gomatrixserverlib.ClientEvent `json:"chunk"`
}

type getJoinedRoomsResponse struct {
	JoinedRooms []string `json:"joined_rooms"`
}

// GetMemberships implements GET /rooms/{roomId}/members
func GetMemberships(
	req *http.Request, device *userapi.Device, roomID string, joinedOnly bool,
	_ *config.Dendrite,
	rsAPI api.RoomserverInternalAPI,
) util.JSONResponse {
	queryReq := api.QueryMembershipsForRoomRequest{
		JoinedOnly: joinedOnly,
		RoomID:     roomID,
		Sender:     device.UserID,
	}
	var queryRes api.QueryMembershipsForRoomResponse
	if err := rsAPI.QueryMembershipsForRoom(req.Context(), &queryReq, &queryRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryMembershipsForRoom failed")
		return jsonerror.InternalServerError()
	}

	if !queryRes.HasBeenInRoom {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: getMembershipResponse{queryRes.JoinEvents},
	}
}

func GetJoinedRooms(
	req *http.Request,
	device *userapi.Device,
	accountsDB accounts.Database,
) util.JSONResponse {
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}
	joinedRooms, err := accountsDB.GetRoomIDsByLocalPart(req.Context(), localpart)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("accountsDB.GetRoomIDsByLocalPart failed")
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: getJoinedRoomsResponse{joinedRooms},
	}
}
