// Copyright 2024 New Vector Ltd.
// Copyright 2024 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"

	"github.com/element-hq/dendrite/roomserver/api"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

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

// GetJoinedMembers implements
//
//	GET /rooms/{roomId}/joined_members
func GetJoinedMembers(
	req *http.Request, device *userapi.Device, roomID string,
	rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	// Validate the userID
	userID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("Device UserID is invalid"),
		}
	}

	// Validate the roomID
	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("RoomID is invalid"),
		}
	}

	// Get the current memberships for the requesting user to determine
	// if they are allowed to query this endpoint.
	queryReq := api.QueryMembershipForUserRequest{
		RoomID: validRoomID.String(),
		UserID: *userID,
	}

	var queryRes api.QueryMembershipForUserResponse
	if queryErr := rsAPI.QueryMembershipForUser(req.Context(), &queryReq, &queryRes); queryErr != nil {
		util.GetLogger(req.Context()).WithError(queryErr).Error("rsAPI.QueryMembershipsForRoom failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if !queryRes.HasBeenInRoom {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
		}
	}

	if !queryRes.IsInRoom {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
		}
	}

	// Get the current membership events
	var membershipsForRoomResp api.QueryMembershipsForRoomResponse
	if err = rsAPI.QueryMembershipsForRoom(req.Context(), &api.QueryMembershipsForRoomRequest{
		JoinedOnly: true,
		RoomID:     validRoomID.String(),
	}, &membershipsForRoomResp); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryEventsByID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	var res getJoinedMembersResponse
	res.Joined = make(map[string]joinedMember)
	for _, ev := range membershipsForRoomResp.JoinEvents {
		var content databaseJoinedMember
		if err := json.Unmarshal(ev.Content, &content); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("failed to unmarshal event content")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		userID, err := rsAPI.QueryUserIDForSender(req.Context(), *validRoomID, spec.SenderID(ev.Sender))
		if err != nil || userID == nil {
			util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryUserIDForSender failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		res.Joined[userID.String()] = joinedMember(content)
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}
