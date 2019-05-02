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

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
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
	userID string,
	roomID string,
) util.JSONResponse {
	Tag := newTag()

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		httputil.LogThenError(req, err)
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "tag",
	)

	if err != nil {
		httputil.LogThenError(req, err)
	}

	dataByte, err := json.Marshal(data)
	if err != nil {
		httputil.LogThenError(req, err)
	}
	err = json.Unmarshal(dataByte, &Tag)

	if err != nil {
		httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: Tag,
	}
}

// PutTag implements PUT /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
func PutTag(
	req *http.Request,
	accountDB *accounts.Database,
	userID string,
	roomID string,
	tag string,
) util.JSONResponse {
	localpart, data, err := obtainSavedTags(req, userID, roomID, accountDB)

	if err != nil {
		return httputil.LogThenError(req, err)
	}
	Tag := newTag()
	var properties gomatrix.TagProperties

	if reqErr := httputil.UnmarshalJSONRequest(req, &properties); reqErr != nil {
		return *reqErr
	}

	if len(data) > 0 {
		dataByte, err := json.Marshal(data)
		if err != nil {
			httputil.LogThenError(req, err)
		}
		if err = json.Unmarshal(dataByte, &Tag); err != nil {
			return httputil.LogThenError(req, err)
		}
	}
	Tag.Tags[tag] = properties
	addDataToDB(req, localpart, roomID, accountDB, Tag)

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteTag implements DELETE /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
func DeleteTag(
	req *http.Request,
	accountDB *accounts.Database,
	userID string,
	roomID string,
	tag string,
) util.JSONResponse {
	localpart, data, err := obtainSavedTags(req, userID, roomID, accountDB)

	if err != nil {
		return httputil.LogThenError(req, err)
	}
	Tag := newTag()

	if len(data) > 0 {
		dataByte, err := json.Marshal(data)
		if err != nil {
			httputil.LogThenError(req, err)
		}
		if err := json.Unmarshal(dataByte, &Tag); err != nil {
			return httputil.LogThenError(req, err)
		}
	} else {
		//Synapse returns a 200 OK response on finding no Tags, same policy is followed here.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	// Check whether the Tag to be deleted exists
	if _, ok := Tag.Tags[tag]; ok {
		delete(Tag.Tags, tag)
	} else {
		//Synapse returns a 200 OK response on finding no Tags, same policy is followed here.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	addDataToDB(req, localpart, roomID, accountDB, Tag)

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
		return "", []gomatrixserverlib.RawJSON{}, err
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "tag",
	)
	if err != nil {
		return "", []gomatrixserverlib.RawJSON{}, err
	}

	return localpart, getContentFromData(data), nil
}

// addDataToDB is a utility function to save the tag data into the DB
func addDataToDB(
	req *http.Request,
	localpart string,
	roomID string,
	accountDB *accounts.Database,
	Tag gomatrix.TagContent,
) {
	newTagData, err := json.Marshal(Tag)
	if err != nil {
		httputil.LogThenError(req, err)
	}
	if err = accountDB.SaveAccountData(
		req.Context(), localpart, roomID, "tag", string(newTagData),
	); err != nil {
		httputil.LogThenError(req, err)
	}
}

// getContentFromData is an utility function to obtain "content" from the ClientEvent 
func getContentFromData(data []gomatrixserverlib.ClientEvent) []gomatrixserverlib.RawJSON {
	var contentData []gomatrixserverlib.RawJSON
	for i:=0 ; i< len(data); i++ {
		contentData = append(contentData,data[i].Content)
	}
	return contentData
}