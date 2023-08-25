// Copyright 2022 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type getJoinedRoomsResponse struct {
	JoinedRooms []string `json:"joined_rooms"`
}

func GetJoinedRooms(
	req *http.Request,
	device *userapi.Device,
	rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("Invalid device user ID")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("internal server error"),
		}
	}

	rooms, err := rsAPI.QueryRoomsForUser(req.Context(), *deviceUserID, "join")
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryRoomsForUser failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("internal server error"),
		}
	}

	var roomIDStrs []string
	if rooms == nil {
		roomIDStrs = []string{}
	} else {
		roomIDStrs = make([]string, len(rooms))
		for i, roomID := range rooms {
			roomIDStrs[i] = roomID.String()
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: getJoinedRoomsResponse{roomIDStrs},
	}
}
