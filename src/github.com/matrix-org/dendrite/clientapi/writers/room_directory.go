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

package writers

import (
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/util"
)

type roomDirectoryVisibility struct {
	Visibility string `json:"visibility"`
}

// GetVisibility implements GET /directory/list/room/{roomID}
func GetVisibility(
	req *http.Request, roomID string, publicRoomAPI api.RoomserverPublicRoomAPI,
) util.JSONResponse {
	queryReq := api.GetRoomVisibilityRequest{roomID}
	var queryRes api.GetRoomVisibilityResponse
	if err := publicRoomAPI.GetRoomVisibility(&queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: roomDirectoryVisibility{queryRes.Visibility},
	}
}

// SetVisibility implements PUT /directory/list/room/{roomID}
// TODO: Check if user has the power leven to edit the room visibility
func SetVisibility(
	req *http.Request, device authtypes.Device, roomID string,
	publicRoomAPI api.RoomserverPublicRoomAPI,
) util.JSONResponse {
	var r roomDirectoryVisibility
	if resErr := httputil.UnmarshalJSONRequest(req, &r); resErr != nil {
		return *resErr
	}

	queryReq := api.SetRoomVisibilityRequest{
		RoomID:     roomID,
		Visibility: r.Visibility,
	}
	var queryRes api.SetRoomVisibilityResponse
	if err := publicRoomAPI.SetRoomVisibility(&queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}

// GetPublicRooms implements GET and POST /publicRooms
func GetPublicRooms(
	req *http.Request, publicRoomAPI api.RoomserverPublicRoomAPI,
) util.JSONResponse {
	queryReq := api.GetPublicRoomsRequest{}
	if fillErr := fillPublicRoomsReq(req, &queryReq); fillErr != nil {
		return *fillErr
	}
	var queryRes api.GetPublicRoomsResponse
	if err := publicRoomAPI.GetPublicRooms(&queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: queryRes,
	}
}

// fillPublicRoomsReq fills the Limit, Since and Filter attributes of a request to the roomserver's
// GetPublicRooms API by parsing the incoming HTTP request
func fillPublicRoomsReq(httpReq *http.Request, queryReq *api.GetPublicRoomsRequest) *util.JSONResponse {
	if httpReq.Method == "GET" {
		limit, err := strconv.Atoi(httpReq.FormValue("limit"))
		// Atoi returns 0 and an error when trying to parse an empty string
		// In that case, we want to assign 0 so we ignore the error
		if err != nil && len(httpReq.FormValue("limit")) > 0 {
			reqErr := httputil.LogThenError(httpReq, err)
			return &reqErr
		}
		queryReq.Limit = int16(limit)
		queryReq.Since = httpReq.FormValue("since")
	} else if httpReq.Method == "POST" {
		if reqErr := httputil.UnmarshalJSONRequest(httpReq, queryReq); reqErr != nil {
			return reqErr
		}
	}

	return nil
}
