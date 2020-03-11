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

package directory

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/util"
)

type roomVisibility struct {
	Visibility string `json:"visibility"`
}

// GetVisibility implements GET /directory/list/room/{roomID}
func GetVisibility(
	req *http.Request, publicRoomsDatabase storage.Database,
	roomID string,
) util.JSONResponse {
	isPublic, err := publicRoomsDatabase.GetRoomVisibility(req.Context(), roomID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("publicRoomsDatabase.GetRoomVisibility failed")
		return jsonerror.InternalServerError()
	}

	var v roomVisibility
	if isPublic {
		v.Visibility = gomatrixserverlib.Public
	} else {
		v.Visibility = "private"
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: v,
	}
}

// SetVisibility implements PUT /directory/list/room/{roomID}
func SetVisibility(
	req *http.Request, publicRoomsDatabase storage.Database, queryAPI api.RoomserverQueryAPI, dev *authtypes.Device,
	roomID string,
) util.JSONResponse {
	queryMembershipReq := api.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: dev.UserID,
	}
	var queryMembershipRes api.QueryMembershipForUserResponse
	err := queryAPI.QueryMembershipForUser(req.Context(), &queryMembershipReq, &queryMembershipRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("could not query membership for user")
		return jsonerror.InternalServerError()
	}
	// Check if user id is in room
	if !queryMembershipRes.IsInRoom {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("user does not belong to room"),
		}
	}
	queryEventsReq := api.QueryLatestEventsAndStateRequest{
		RoomID: roomID,
		StateToFetch: []gomatrixserverlib.StateKeyTuple{{
			EventType: gomatrixserverlib.MRoomPowerLevels,
			StateKey:  "",
		}},
	}
	var queryEventsRes api.QueryLatestEventsAndStateResponse
	err = queryAPI.QueryLatestEventsAndState(req.Context(), &queryEventsReq, &queryEventsRes)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("could not query events from room")
		return jsonerror.InternalServerError()
	}
	power, _ := gomatrixserverlib.NewPowerLevelContentFromEvent(queryEventsRes.StateEvents[0])

	//Check if the user's power is greater than power required to change m.room.aliases event
	if power.UserLevel(dev.UserID) < power.EventLevel(gomatrixserverlib.MRoomAliases, true) {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID doesn't have power level to change visibility"),
		}
	}

	var v roomVisibility
	if reqErr := httputil.UnmarshalJSONRequest(req, &v); reqErr != nil {
		return *reqErr
	}

	isPublic := v.Visibility == gomatrixserverlib.Public
	if err := publicRoomsDatabase.SetRoomVisibility(req.Context(), isPublic, roomID); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("publicRoomsDatabase.SetRoomVisibility failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
