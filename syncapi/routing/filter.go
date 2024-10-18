// Copyright 2024 New Vector Ltd.
// Copyright 2017 Jan Christian Grünhage
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"

	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/sync"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// GetFilter implements GET /_matrix/client/r0/user/{userId}/filter/{filterId}
func GetFilter(
	req *http.Request, device *api.Device, syncDB storage.Database, userID string, filterID string,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Cannot get filters for other users"),
		}
	}
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	filter := synctypes.DefaultFilter()
	if err := syncDB.GetFilter(req.Context(), &filter, localpart, filterID); err != nil {
		//TODO better error handling. This error message is *probably* right,
		// but if there are obscure db errors, this will also be returned,
		// even though it is not correct.
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.NotFound("No such filter"),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: filter,
	}
}

type filterResponse struct {
	FilterID string `json:"filter_id"`
}

// PutFilter implements
//
//	POST /_matrix/client/r0/user/{userId}/filter
func PutFilter(
	req *http.Request, device *api.Device, syncDB storage.Database, userID string,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Cannot create filters for other users"),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	var filter synctypes.Filter

	defer req.Body.Close() // nolint:errcheck
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The request body could not be read. " + err.Error()),
		}
	}

	if err = json.Unmarshal(body, &filter); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}
	// the filter `limit` is `int` which defaults to 0 if not set which is not what we want. We want to use the default
	// limit if it is unset, which is what this does.
	limitRes := gjson.GetBytes(body, "room.timeline.limit")
	if !limitRes.Exists() {
		util.GetLogger(req.Context()).Infof("missing timeline limit, using default")
		filter.Room.Timeline.Limit = sync.DefaultTimelineLimit
	}

	// Validate generates a user-friendly error
	if err = filter.Validate(); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("Invalid filter: " + err.Error()),
		}
	}

	filterID, err := syncDB.PutFilter(req.Context(), localpart, &filter)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("syncDB.PutFilter failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: filterResponse{FilterID: filterID},
	}
}
