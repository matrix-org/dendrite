// Copyright 2017 Jan Christian Grünhage
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
	"io"
	"net/http"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/sync"
	"github.com/matrix-org/dendrite/userapi/api"
)

// GetFilter implements GET /_matrix/client/r0/user/{userId}/filter/{filterId}
func GetFilter(
	req *http.Request, device *api.Device, syncDB storage.Database, userID string, filterID string,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Cannot get filters for other users"),
		}
	}
	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	filter := gomatrixserverlib.DefaultFilter()
	if err := syncDB.GetFilter(req.Context(), &filter, localpart, filterID); err != nil {
		//TODO better error handling. This error message is *probably* right,
		// but if there are obscure db errors, this will also be returned,
		// even though it is not correct.
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotFound("No such filter"),
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
			JSON: jsonerror.Forbidden("Cannot create filters for other users"),
		}
	}

	localpart, _, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("gomatrixserverlib.SplitID failed")
		return jsonerror.InternalServerError()
	}

	var filter gomatrixserverlib.Filter

	defer req.Body.Close() // nolint:errcheck
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be read. " + err.Error()),
		}
	}

	if err = json.Unmarshal(body, &filter); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be decoded into valid JSON. " + err.Error()),
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
			JSON: jsonerror.BadJSON("Invalid filter: " + err.Error()),
		}
	}

	filterID, err := syncDB.PutFilter(req.Context(), localpart, &filter)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("syncDB.PutFilter failed")
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: filterResponse{FilterID: filterID},
	}
}
