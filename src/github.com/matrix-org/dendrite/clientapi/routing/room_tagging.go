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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// MewMTag creates and returns a new MTag type variable
func NewMTag() common.MTag {
	return common.MTag{
		Tags: make(map[string]common.TagProperties),
	}
}

// GetTag implements GET /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags
func GetTag(req *http.Request, accountDB *accounts.Database, userID string, roomID string) util.JSONResponse {

	if req.Method != http.MethodGet {
		return util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}

	mtag := NewMTag()

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "m.tag",
	)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	dataByte, _ := json.Marshal(data)
	err = json.Unmarshal(dataByte, &mtag)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: mtag,
	}
}

// PutTag implements PUT /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
func PutTag(req *http.Request, accountDB *accounts.Database, userID string, roomID string, tag string) util.JSONResponse {

	if req.Method != http.MethodPut {
		return util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	//Check for existing entries of tags for this ROOM ID and localpart
	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "m.tag",
	)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	mtag := NewMTag()

	var properties common.TagProperties

	if reqErr := httputil.UnmarshalJSONRequest(req, &properties); reqErr != nil {
		return *reqErr
	}

	if len(data) > 0 {
		dataByte, _ := json.Marshal(data)
		err = json.Unmarshal(dataByte, &mtag)
		if err != nil {
			return httputil.LogThenError(req, err)
		}
	}

	mtag.Tags[tag] = properties

	newTagData, _ := json.Marshal(mtag)

	if err := accountDB.SaveAccountData(
		req.Context(), localpart, roomID, "m.tag", string(newTagData),
	); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteTag implements DELETE /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
func DeleteTag(req *http.Request, accountDB *accounts.Database, userID string, roomID string, tag string) util.JSONResponse {

	if req.Method != http.MethodDelete {
		return util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: jsonerror.NotFound("Bad method"),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "m.tag",
	)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	mtag := NewMTag()

	if len(data) > 0 {
		dataByte, _ := json.Marshal(data)
		err = json.Unmarshal(dataByte, &mtag)
		if err != nil {
			return httputil.LogThenError(req, err)
		}
	} else {
		//Error indicating there is no Tag data
		return util.JSONResponse{}
	}

	//Check whether the Tag to be deleted exists
	if _, ok := mtag.Tags[tag]; ok {
		delete(mtag.Tags, tag) //Deletion completed
	} else {
		//Error indicating that there is no Tag to delete
		return util.JSONResponse{}
	}

	newTagData, _ := json.Marshal(mtag)

	if err := accountDB.SaveAccountData(
		req.Context(), localpart, roomID, "m.tag", string(newTagData),
	); err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
