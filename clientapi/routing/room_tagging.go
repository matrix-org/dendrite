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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/accounts"
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
	accountDB accounts.Database,
	device *api.Device,
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
		util.GetLogger(req.Context()).WithError(err).Error("obtainSavedTags failed")
		return jsonerror.InternalServerError()
	}

	if data == nil {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: data,
	}
}

// PutTag implements PUT /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
// Put functionality works by getting existing data from the DB (if any), adding
// the tag to the "map" and saving the new "map" to the DB
func PutTag(
	req *http.Request,
	accountDB accounts.Database,
	device *api.Device,
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
		util.GetLogger(req.Context()).WithError(err).Error("obtainSavedTags failed")
		return jsonerror.InternalServerError()
	}

	var tagContent gomatrix.TagContent
	if data != nil {
		if err = json.Unmarshal(data, &tagContent); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("json.Unmarshal failed")
			return jsonerror.InternalServerError()
		}
	} else {
		tagContent = newTag()
	}
	tagContent.Tags[tag] = properties
	if err = saveTagData(req, localpart, roomID, accountDB, tagContent); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("saveTagData failed")
		return jsonerror.InternalServerError()
	}

	// Send data to syncProducer in order to inform clients of changes
	// Run in a goroutine in order to prevent blocking the tag request response
	go func() {
		if err := syncProducer.SendData(userID, roomID, "m.tag"); err != nil {
			logrus.WithError(err).Error("Failed to send m.tag account data update to syncapi")
		}
	}()

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// DeleteTag implements DELETE /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
// Delete functionality works by obtaining the saved tags, removing the intended tag from
// the "map" and then saving the new "map" in the DB
func DeleteTag(
	req *http.Request,
	accountDB accounts.Database,
	device *api.Device,
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
		util.GetLogger(req.Context()).WithError(err).Error("obtainSavedTags failed")
		return jsonerror.InternalServerError()
	}

	// If there are no tags in the database, exit
	if data == nil {
		// Spec only defines 200 responses for this endpoint so we don't return anything else.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}

	var tagContent gomatrix.TagContent
	err = json.Unmarshal(data, &tagContent)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("json.Unmarshal failed")
		return jsonerror.InternalServerError()
	}

	// Check whether the tag to be deleted exists
	if _, ok := tagContent.Tags[tag]; ok {
		delete(tagContent.Tags, tag)
	} else {
		// Spec only defines 200 responses for this endpoint so we don't return anything else.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	if err = saveTagData(req, localpart, roomID, accountDB, tagContent); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("saveTagData failed")
		return jsonerror.InternalServerError()
	}

	// Send data to syncProducer in order to inform clients of changes
	// Run in a goroutine in order to prevent blocking the tag request response
	go func() {
		if err := syncProducer.SendData(userID, roomID, "m.tag"); err != nil {
			logrus.WithError(err).Error("Failed to send m.tag account data update to syncapi")
		}
	}()

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}

// obtainSavedTags gets all tags scoped to a userID and roomID
// from the database
func obtainSavedTags(
	req *http.Request,
	userID string,
	roomID string,
	accountDB accounts.Database,
) (string, json.RawMessage, error) {
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return "", nil, err
	}

	data, err := accountDB.GetAccountDataByType(
		req.Context(), localpart, roomID, "m.tag",
	)

	return localpart, data, err
}

// saveTagData saves the provided tag data into the database
func saveTagData(
	req *http.Request,
	localpart string,
	roomID string,
	accountDB accounts.Database,
	Tag gomatrix.TagContent,
) error {
	newTagData, err := json.Marshal(Tag)
	if err != nil {
		return err
	}

	return accountDB.SaveAccountData(req.Context(), localpart, roomID, "m.tag", json.RawMessage(newTagData))
}
