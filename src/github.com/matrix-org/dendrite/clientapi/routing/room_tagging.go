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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetTag implements GET /_matrix/client/r0/user/{userId}/rooms/{roomId}/tags
func GetTag(req *http.Request, userId string, roomId string) util.JSONResponse {

	if req.Method != http.MethodPGet {
		return util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, "ROOM_ID", "m.tag",
	)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: tags,
	}
}

// PutTag implements PUT /_matrix/client/r0/user/{userId}/rooms/{roomId}/tags/{tag}
func PutTag(req *http.Request, userId string, roomId string, tag common.Tag) util.JSONResponse {

	if req.Method != http.MethodPut {
		return util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', "USER_ID")
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	//Check for existing entries of tags for this ROOM ID and localpart
	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, "ROOM_ID", "m.tag",
	)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	outInterface := map[string]interface{}{}
	json.Unmarshal([]byte(data), &outInterface)

	if len(data) > 0 {
		if outInterface[tag.Name] != nil {
			// Error saying this Tag already exists
		}
	}

	outInterface[tag.Name] = tag.Order

	newTagData, _ := json.Marshal(outInterface)

	if err := accountDB.SaveAccountData(
		req.Context(), localpart, "ROOM_ID", "m.tag", newTagData,
	); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: {},
	}
}

// DeleteTag implements DELETE /_matrix/client/r0/user/{userId}/rooms/{roomId}/tags/{tag}
func DeleteTag(req *http.Request, userId string, roomId string, tag string) util.JSONResponse {

	if req.Method != http.MethodDelete {
		return util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', "USER_ID")
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: {},
	}
}
