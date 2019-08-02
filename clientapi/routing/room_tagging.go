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

	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// newTag creates and returns a new gomatrix.TagContent
func newTag() gomatrix.TagContent {
	return gomatrix.TagContent{
		Tags: make(map[string]gomatrix.TagProperties),
	}
}

// GetTags implements GET /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags
func GetTags(
	req *http.Request,
	accountDB *accounts.Database,
	device *authtypes.Device,
	userID string,
	roomID string,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot retrieve another user's tags"),
		}
	}

	_, data, err := obtainSavedTags(req, userID, roomID, accountDB)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if len(data) == 0 {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	go func() {
		if err := syncProducer.SendData(userID, roomID, "m.tag"); err != nil {
			logrus.WithError(err).Error("Incremental sync operation failed")
		}
	}()

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// PutTag implements PUT /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
// Put functionality works by getting existing data from the DB (if any), adding
// the tag on to the "map" and saving the new "map" onto the DB
func PutTag(
	req *http.Request,
	accountDB *accounts.Database,
	device *authtypes.Device,
	userID string,
	roomID string,
	tag string,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot modify another user's tags"),
		}
	}

	var properties gomatrix.TagProperties
	if reqErr := httputil.UnmarshalJSONRequest(req, &properties); reqErr != nil {
		return *reqErr
	}

	localpart, data, err := obtainSavedTags(req, userID, roomID, accountDB)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	tagContent := newTag()
	if len(data) > 0 {
		if err = json.Unmarshal(data[0].Content, &tagContent); err != nil {
			return httputil.LogThenError(req, err)
		}
	}
	tagContent.Tags[tag] = properties
	if err = saveTagData(req, localpart, roomID, accountDB, tagContent); err != nil {
		return httputil.LogThenError(req, err)
	}

	go func() {
		if err := syncProducer.SendData(userID, roomID, "m.tag"); err != nil {
			logrus.WithError(err).Error("Incremental sync operation failed")
		}
	}()

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteTag implements DELETE /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
// Delete functionality works by obtaining the saved Tags, removing the intended tag from
// the "map" and then saving the new "map" in the DB
func DeleteTag(
	req *http.Request,
	accountDB *accounts.Database,
	device *authtypes.Device,
	userID string,
	roomID string,
	tag string,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot modify another user's tags"),
		}
	}

	localpart, data, err := obtainSavedTags(req, userID, roomID, accountDB)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	// If there are no tags in the database, exit
	if len(data) == 0 {
		//Specifications mentions a 200 OK response is returned on finding no Tags, same policy is followed here.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	tagContent := newTag()
	err = json.Unmarshal(data[0].Content, &tagContent)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	// Check whether the Tag to be deleted exists
	if _, ok := tagContent.Tags[tag]; ok {
		delete(tagContent.Tags, tag)
	} else {
		//Specifications mentions a 200 OK response is returned on finding no Tags, same policy is followed here.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	if err = saveTagData(req, localpart, roomID, accountDB, tagContent); err != nil {
		return httputil.LogThenError(req, err)
	}

	go func() {
		if err := syncProducer.SendData(userID, roomID, "m.tag"); err != nil {
			logrus.WithError(err).Error("Incremental sync operation failed")
		}
	}()

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// obtainSavedTags gets all the tags saved in the DB
func obtainSavedTags(
	req *http.Request,
	userID string,
	roomID string,
	accountDB *accounts.Database,
) (string, []gomatrixserverlib.ClientEvent, error) {
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return "", nil, err
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "m.tag",
	)
	if err != nil {
		return "", nil, err
	}

	return localpart, data, nil
}

// saveTagData saves the tag data into the DB
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

	return accountDB.SaveAccountData(req.Context(), localpart, roomID, "m.tag", string(newTagData))
}
