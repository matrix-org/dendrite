// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/matrix-org/util"

	"github.com/element-hq/dendrite/roomserver/api"
	userapi "github.com/element-hq/dendrite/userapi/api"
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
