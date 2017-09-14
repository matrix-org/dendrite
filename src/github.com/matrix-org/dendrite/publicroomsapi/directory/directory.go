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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"

	"github.com/matrix-org/util"
)

type roomVisibility struct {
	Visibility string `json:"visibility"`
}

// GetVisibility implements GET /directory/list/room/{roomID}
func GetVisibility(
	req *http.Request, publicRoomsDatabase *storage.PublicRoomsServerDatabase,
	roomID string,
) util.JSONResponse {
	isPublic, err := publicRoomsDatabase.GetRoomVisibility(req.Context(), roomID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	var v roomVisibility
	if isPublic {
		v.Visibility = "public"
	} else {
		v.Visibility = "private"
	}

	return util.JSONResponse{
		Code: 200,
		JSON: v,
	}
}

// SetVisibility implements PUT /directory/list/room/{roomID}
// TODO: Check if user has the power level to edit the room visibility
func SetVisibility(
	req *http.Request, publicRoomsDatabase *storage.PublicRoomsServerDatabase,
	roomID string,
) util.JSONResponse {
	var v roomVisibility
	if reqErr := httputil.UnmarshalJSONRequest(req, &v); reqErr != nil {
		return *reqErr
	}

	isPublic := v.Visibility == "public"
	if err := publicRoomsDatabase.SetRoomVisibility(req.Context(), isPublic, roomID); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}
