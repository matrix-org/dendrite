// Copyright 2024 New Vector Ltd.
// Copyright 2019 Sumukha PK
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"

	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/clientapi/producers"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetTags implements GET /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags
func GetTags(
	req *http.Request,
	userAPI api.ClientUserAPI,
	device *api.Device,
	userID string,
	roomID string,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Cannot retrieve another user's tags"),
		}
	}

	tagContent, err := obtainSavedTags(req, userID, roomID, userAPI)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("obtainSavedTags failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: tagContent,
	}
}

// PutTag implements PUT /_matrix/client/r0/user/{userID}/rooms/{roomID}/tags/{tag}
// Put functionality works by getting existing data from the DB (if any), adding
// the tag to the "map" and saving the new "map" to the DB
func PutTag(
	req *http.Request,
	userAPI api.ClientUserAPI,
	device *api.Device,
	userID string,
	roomID string,
	tag string,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Cannot modify another user's tags"),
		}
	}

	var properties gomatrix.TagProperties
	if reqErr := httputil.UnmarshalJSONRequest(req, &properties); reqErr != nil {
		return *reqErr
	}

	tagContent, err := obtainSavedTags(req, userID, roomID, userAPI)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("obtainSavedTags failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if tagContent.Tags == nil {
		tagContent.Tags = make(map[string]gomatrix.TagProperties)
	}
	tagContent.Tags[tag] = properties

	if err = saveTagData(req, userID, roomID, userAPI, tagContent); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("saveTagData failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

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
	userAPI api.ClientUserAPI,
	device *api.Device,
	userID string,
	roomID string,
	tag string,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {

	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Cannot modify another user's tags"),
		}
	}

	tagContent, err := obtainSavedTags(req, userID, roomID, userAPI)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("obtainSavedTags failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
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

	if err = saveTagData(req, userID, roomID, userAPI, tagContent); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("saveTagData failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

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
	userAPI api.ClientUserAPI,
) (tags gomatrix.TagContent, err error) {
	dataReq := api.QueryAccountDataRequest{
		UserID:   userID,
		RoomID:   roomID,
		DataType: "m.tag",
	}
	dataRes := api.QueryAccountDataResponse{}
	err = userAPI.QueryAccountData(req.Context(), &dataReq, &dataRes)
	if err != nil {
		return
	}
	data, ok := dataRes.RoomAccountData[roomID]["m.tag"]
	if !ok {
		return
	}
	if err = json.Unmarshal(data, &tags); err != nil {
		return
	}
	return tags, nil
}

// saveTagData saves the provided tag data into the database
func saveTagData(
	req *http.Request,
	userID string,
	roomID string,
	userAPI api.ClientUserAPI,
	Tag gomatrix.TagContent,
) error {
	newTagData, err := json.Marshal(Tag)
	if err != nil {
		return err
	}
	dataReq := api.InputAccountDataRequest{
		UserID:      userID,
		RoomID:      roomID,
		DataType:    "m.tag",
		AccountData: json.RawMessage(newTagData),
	}
	dataRes := api.InputAccountDataResponse{}
	return userAPI.InputAccountData(req.Context(), &dataReq, &dataRes)
}
