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

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type getMembershipResponse struct {
	Chunk []gomatrixserverlib.ClientEvent `json:"chunk"`
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

// GetMemberships implements
//
//	GET /rooms/{roomId}/members
//	GET /rooms/{roomId}/joined_members
func GetMemberships(
	req *http.Request, device *userapi.Device, roomID string,
	syncDB storage.Database, rsAPI api.SyncRoomserverAPI,
	joinedOnly bool, membership, notMembership *string, at string,
) util.JSONResponse {
	queryReq := api.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: device.UserID,
	}

	var queryRes api.QueryMembershipForUserResponse
	if err := rsAPI.QueryMembershipForUser(req.Context(), &queryReq, &queryRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryMembershipsForRoom failed")
		return jsonerror.InternalServerError()
	}

	if !queryRes.HasBeenInRoom {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
		}
	}

	if joinedOnly && !queryRes.IsInRoom {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
		}
	}

	db, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		return jsonerror.InternalServerError()
	}

	atToken, err := types.NewTopologyTokenFromString(at)
	if err != nil {
		if queryRes.HasBeenInRoom && !queryRes.IsInRoom {
			// If you have left the room then this will be the members of the room when you left.
			atToken, err = db.EventPositionInTopology(req.Context(), queryRes.EventID)
		} else {
			// If you are joined to the room then this will be the current members of the room.
			atToken, err = db.MaxTopologicalPosition(req.Context(), roomID)
		}
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("unable to get 'atToken'")
			return jsonerror.InternalServerError()
		}
	}

	eventIDs, err := db.SelectMemberships(req.Context(), roomID, atToken, membership, notMembership)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("db.SelectMemberships failed")
		return jsonerror.InternalServerError()
	}

	qryRes := &api.QueryEventsByIDResponse{}
	if err := rsAPI.QueryEventsByID(req.Context(), &api.QueryEventsByIDRequest{EventIDs: eventIDs}, qryRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryEventsByID failed")
		return jsonerror.InternalServerError()
	}

	result := qryRes.Events

	if joinedOnly {
		var res getJoinedMembersResponse
		res.Joined = make(map[string]joinedMember)
		for _, ev := range result {
			var content databaseJoinedMember
			if err := json.Unmarshal(ev.Content(), &content); err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("failed to unmarshal event content")
				return jsonerror.InternalServerError()
			}
			res.Joined[ev.Sender()] = joinedMember(content)
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: res,
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: getMembershipResponse{gomatrixserverlib.HeaderedToClientEvents(result, gomatrixserverlib.FormatAll)},
	}
}
