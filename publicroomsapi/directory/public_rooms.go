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

package directory

import (
	"context"
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type PublicRoomReq struct {
	Since  string `json:"since,omitempty"`
	Limit  int16  `json:"limit,omitempty"`
	Filter filter `json:"filter,omitempty"`
}

type filter struct {
	SearchTerms string `json:"generic_search_term,omitempty"`
}

// GetPostPublicRooms implements GET and POST /publicRooms
func GetPostPublicRooms(
	req *http.Request, publicRoomDatabase storage.Database,
) util.JSONResponse {
	var request PublicRoomReq
	if fillErr := fillPublicRoomsReq(req, &request); fillErr != nil {
		return *fillErr
	}
	response, err := PublicRooms(req.Context(), request, publicRoomDatabase)
	if err != nil {
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}

func PublicRooms(ctx context.Context, request PublicRoomReq, publicRoomDatabase storage.Database) (*gomatrixserverlib.RespPublicRooms, error) {
	var response gomatrixserverlib.RespPublicRooms
	var limit int16
	var offset int64
	limit = request.Limit
	offset, err := strconv.ParseInt(request.Since, 10, 64)
	// ParseInt returns 0 and an error when trying to parse an empty string
	// In that case, we want to assign 0 so we ignore the error
	if err != nil && len(request.Since) > 0 {
		util.GetLogger(ctx).WithError(err).Error("strconv.ParseInt failed")
		return nil, err
	}

	est, err := publicRoomDatabase.CountPublicRooms(ctx)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("publicRoomDatabase.CountPublicRooms failed")
		return nil, err
	}
	response.TotalRoomCountEstimate = int(est)

	if offset > 0 {
		response.PrevBatch = strconv.Itoa(int(offset) - 1)
	}
	nextIndex := int(offset) + int(limit)
	if response.TotalRoomCountEstimate > nextIndex {
		response.NextBatch = strconv.Itoa(nextIndex)
	}

	if response.Chunk, err = publicRoomDatabase.GetPublicRooms(
		ctx, offset, limit, request.Filter.SearchTerms,
	); err != nil {
		util.GetLogger(ctx).WithError(err).Error("publicRoomDatabase.GetPublicRooms failed")
		return nil, err
	}

	return &response, nil
}

// fillPublicRoomsReq fills the Limit, Since and Filter attributes of a GET or POST request
// on /publicRooms by parsing the incoming HTTP request
// Filter is only filled for POST requests
func fillPublicRoomsReq(httpReq *http.Request, request *PublicRoomReq) *util.JSONResponse {
	if httpReq.Method == http.MethodGet {
		limit, err := strconv.Atoi(httpReq.FormValue("limit"))
		// Atoi returns 0 and an error when trying to parse an empty string
		// In that case, we want to assign 0 so we ignore the error
		if err != nil && len(httpReq.FormValue("limit")) > 0 {
			util.GetLogger(httpReq.Context()).WithError(err).Error("strconv.Atoi failed")
			reqErr := jsonerror.InternalServerError()
			return &reqErr
		}
		request.Limit = int16(limit)
		request.Since = httpReq.FormValue("since")
		return nil
	} else if httpReq.Method == http.MethodPost {
		return httputil.UnmarshalJSONRequest(httpReq, request)
	}

	return &util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: jsonerror.NotFound("Bad method"),
	}
}
