// Copyright 2020 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"

	"github.com/matrix-org/util"
)

// GetCapabilities returns information about the server's supported feature set
// and other relevant capabilities to an authenticated user.
func GetCapabilities(
	req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI,
) util.JSONResponse {
	roomVersionsQueryReq := roomserverAPI.QueryRoomVersionCapabilitiesRequest{}
	roomVersionsQueryRes := roomserverAPI.QueryRoomVersionCapabilitiesResponse{}
	if err := rsAPI.QueryRoomVersionCapabilities(
		req.Context(),
		&roomVersionsQueryReq,
		&roomVersionsQueryRes,
	); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("queryAPI.QueryRoomVersionCapabilities failed")
		return jsonerror.InternalServerError()
	}

	response := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"m.change_password": map[string]bool{
				"enabled": true,
			},
			"m.room_versions": roomVersionsQueryRes,
		},
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}
