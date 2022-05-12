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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
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

// https://matrix.org/docs/spec/client_server/r0.6.0#get-matrix-client-r0-rooms-roomid-joined-members
type getJoinedMembersResponse struct {
	Joined map[string]joinedMember `json:"joined"`
}

type joinedMember struct {
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// The database stores 'displayname' without an underscore.
// Deserialize into this and then change to the actual API response
type databaseJoinedMember struct {
	DisplayName string `json:"displayname"`
	AvatarURL   string `json:"avatar_url"`
}

// GetMemberships implements GET /rooms/{roomId}/members
func GetMemberships(
	req *http.Request, device *userapi.Device, roomID string, joinedOnly bool,
	_ *config.ClientAPI,
	rsAPI api.ClientRoomserverAPI,
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

	if joinedOnly {
		var res getJoinedMembersResponse
		res.Joined = make(map[string]joinedMember)
		for _, ev := range queryRes.JoinEvents {
			var content databaseJoinedMember
			if err := json.Unmarshal(ev.Content, &content); err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("failed to unmarshal event content")
				return jsonerror.InternalServerError()
			}
			res.Joined[ev.Sender] = joinedMember(content)
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: res,
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
	rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	var res api.QueryRoomsForUserResponse
	err := rsAPI.QueryRoomsForUser(req.Context(), &api.QueryRoomsForUserRequest{
		UserID:         device.UserID,
		WantMembership: "join",
	}, &res)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("QueryRoomsForUser failed")
		return jsonerror.InternalServerError()
	}
	if res.RoomIDs == nil {
		res.RoomIDs = []string{}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: getJoinedRoomsResponse{res.RoomIDs},
	}
}
