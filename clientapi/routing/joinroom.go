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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func JoinRoomByIDOrAlias(
	req *http.Request,
	device *api.Device,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	accountDB accounts.Database,
	roomIDOrAlias string,
) util.JSONResponse {
	// Prepare to ask the roomserver to perform the room join.
	joinReq := roomserverAPI.PerformJoinRequest{
		RoomIDOrAlias: roomIDOrAlias,
		UserID:        device.UserID,
	}
	joinRes := roomserverAPI.PerformJoinResponse{}

	// If content was provided in the request then incude that
	// in the request. It'll get used as a part of the membership
	// event content.
	if err := httputil.UnmarshalJSONRequest(req, &joinReq.Content); err != nil {
		return *err
	}

	// Work out our localpart for the client profile request.
	localpart, _, err := gomatrixserverlib.SplitID('@', device.UserID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
	} else {
		// Request our profile content to populate the request content with.
		profile, err := accountDB.GetProfileByLocalpart(req.Context(), localpart)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("accountDB.GetProfileByLocalpart failed")
		} else {
			joinReq.Content["displayname"] = profile.DisplayName
			joinReq.Content["avatar_url"] = profile.AvatarURL
		}
	}

	// Ask the roomserver to perform the join.
	if err := rsAPI.PerformJoin(req.Context(), &joinReq, &joinRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		// TODO: Put the response struct somewhere internal.
		JSON: struct {
			RoomID string `json:"room_id"`
		}{joinRes.RoomID},
	}
}
