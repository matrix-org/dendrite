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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type response struct {
	Chunk []gomatrixserverlib.ClientEvent `json:"chunk"`
}

// GetMemberships implements GET /rooms/{roomId}/members
func GetMemberships(
	req *http.Request, device *authtypes.Device, roomID string, joinedOnly bool,
	_ config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
) util.JSONResponse {
	queryReq := api.QueryMembershipsForRoomRequest{
		JoinedOnly: joinedOnly,
		RoomID:     roomID,
		Sender:     device.UserID,
	}
	var queryRes api.QueryMembershipsForRoomResponse
	if err := queryAPI.QueryMembershipsForRoom(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	if !queryRes.HasBeenInRoom {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response{queryRes.JoinEvents},
	}
}
