// Copyright 2019 Sumukha PK
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

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// newTag creates and returns a new Tag type
func newTag() gomatrix.TagContent {
	return gomatrix.TagContent{
		Tags: make(map[string]gomatrix.TagProperties),
	}
}

// GetTag implements GET /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags
func GetTag(
	req *http.Request,
	accountDB *accounts.Database,
	device *authtypes.Device,
	userID string,
	roomID string,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot retrieve another user's typing state"),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "tag",
	)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	dataByte, err := json.Marshal(data)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	tagContent := newTag()
	err = json.Unmarshal(dataByte, &tagContent)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: tagContent,
	}
}

// PutTag implements PUT /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
func PutTag(
	req *http.Request,
	accountDB *accounts.Database,
	device *authtypes.Device,
	userID string,
	roomID string,
	tag string,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot modify another user's typing state"),
		}
	}

	localpart, data, err := obtainSavedTags(req, userID, roomID, accountDB)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	var properties gomatrix.TagProperties

	if reqErr := httputil.UnmarshalJSONRequest(req, &properties); reqErr != nil {
		return *reqErr
	}

	tagContent := newTag()
	var dataByte []byte
	if len(data) > 0 {
		dataByte, err = json.Marshal(data)
		if err != nil {
			return httputil.LogThenError(req, err)
		}
		if err = json.Unmarshal(dataByte, &tagContent); err != nil {
			return httputil.LogThenError(req, err)
		}
	}
	tagContent.Tags[tag] = properties
	err = saveTagData(req, localpart, roomID, accountDB, tagContent)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteTag implements DELETE /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
func DeleteTag(
	req *http.Request,
	accountDB *accounts.Database,
	device *authtypes.Device,
	userID string,
	roomID string,
	tag string,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot delete another user's typing state"),
		}
	}

	localpart, data, err := obtainSavedTags(req, userID, roomID, accountDB)

	if err != nil {
		return httputil.LogThenError(req, err)
	}
	tagContent := newTag()

	// If there are no tags in the database, exit.
	if len(data) == 0 {
		//Synapse returns a 200 OK response on finding no Tags, same policy is followed here.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	byteData, err := json.Marshal(data)
	if err != nil {
		return httputil.LogThenError(req, err)
	}
	if err = json.Unmarshal(byteData, &tagContent); err != nil {
		return httputil.LogThenError(req, err)
	}

	// Check whether the Tag to be deleted exists
	if _, ok := tagContent.Tags[tag]; ok {
		delete(tagContent.Tags, tag)
	} else {
		//Synapse returns a 200 OK response on finding no Tags, same policy is followed here.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	err = saveTagData(req, localpart, roomID, accountDB, Tag)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// obtainSavedTags is a utility function to get all the tags saved in the DB
func obtainSavedTags(
	req *http.Request,
	userID string,
	roomID string,
	accountDB *accounts.Database,
) (string, []gomatrixserverlib.RawJSON, error) {
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return "", nil, err
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "tag",
	)
	if err != nil {
		return "", nil, err
	}

	return localpart, extractEventContents(data), nil
}

// saveTagData is a utility function to save the tag data into the DB
func saveTagData(
	req *http.Request,
	localpart string,
	roomID string,
	accountDB *accounts.Database,
	Tag gomatrix.TagContent,
) error {
	newTagData, err := json.Marshal(Tag)
	if err != nil {
		return err
	}

	return accountDB.SaveAccountData(req.Context(), localpart, roomID, "tag", string(newTagData))
}

// extractEventContents is an utility function to obtain "content" from the ClientEvent
func extractEventContents(data []gomatrixserverlib.ClientEvent) []gomatrixserverlib.RawJSON {
	contentData := make([]gomatrixserverlib.RawJSON, 0, len(data))
	for i := 0; i < len(data); i++ {
		contentData = append(contentData, data[i].Content)
	}
	return contentData
}
