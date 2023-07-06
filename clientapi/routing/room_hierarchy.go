// Copyright 2023 The Matrix.org Foundation C.I.C.
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
	"net/http"
	"strconv"
	"sync"

	"github.com/google/uuid"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type RoomHierarchyPaginationCache struct {
	cache map[string]roomserverAPI.CachedRoomHierarchyWalker
	mu    sync.Mutex
}

func NewRoomHierarchyPaginationCache() RoomHierarchyPaginationCache {
	return RoomHierarchyPaginationCache{
		cache: map[string]roomserverAPI.CachedRoomHierarchyWalker{},
	}
}

func (c *RoomHierarchyPaginationCache) Get(token string) roomserverAPI.CachedRoomHierarchyWalker {
	c.mu.Lock()
	defer c.mu.Unlock()
	line := c.cache[token]
	return line
}

func (c *RoomHierarchyPaginationCache) AddLine(line roomserverAPI.CachedRoomHierarchyWalker) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	token := uuid.NewString()
	c.cache[token] = line
	return token
}

func QueryRoomHierarchy(req *http.Request, device *userapi.Device, roomIDStr string, rsAPI roomserverAPI.ClientRoomserverAPI, paginationCache *RoomHierarchyPaginationCache) util.JSONResponse {
	parsedRoomID, err := spec.NewRoomID(roomIDStr)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.InvalidParam("room is unknown/forbidden"),
		}
	}
	roomID := *parsedRoomID

	suggestedOnly := false // Defaults to false (spec-defined)
	switch req.URL.Query().Get("suggested_only") {
	case "true":
		suggestedOnly = true
	case "false":
	case "": // Empty string is returned when query param is not set
	default:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("query parameter 'suggested_only', if set, must be 'true' or 'false'"),
		}
	}

	limit := 1000 // Default to 1000
	limitStr := req.URL.Query().Get("limit")
	if limitStr != "" {
		maybeLimit, err := strconv.Atoi(limitStr)
		if err != nil || maybeLimit < 0 {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("query parameter 'limit', if set, must be a positive integer"),
			}
		}
		limit = maybeLimit
		if limit > 1000 {
			limit = 1000 // Maximum limit of 1000
		}
	}

	maxDepth := -1 // '-1' representing no maximum depth
	maxDepthStr := req.URL.Query().Get("max_depth")
	if maxDepthStr != "" {
		maybeMaxDepth, err := strconv.Atoi(maxDepthStr)
		if err != nil || maybeMaxDepth < 0 {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("query parameter 'max_depth', if set, must be a positive integer"),
			}
		}
		maxDepth = maybeMaxDepth
	}

	from := req.URL.Query().Get("from")

	var walker roomserverAPI.RoomHierarchyWalker
	if from == "" { // No pagination token provided, so start new hierarchy walker
		walker = rsAPI.QueryRoomHierarchy(req.Context(), types.NewDeviceNotServerName(*device), roomID, suggestedOnly, maxDepth)
	} else { // Attempt to resume cached walker
		cachedWalker := paginationCache.Get(from)

		if cachedWalker == nil || !cachedWalker.ValidateParams(suggestedOnly, maxDepth) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("pagination not found for provided token ('from') with given 'max_depth', 'suggested_only' and room ID"),
			}
		}

		walker = cachedWalker.GetWalker()
	}

	discoveredRooms, err := walker.NextPage(limit)

	if err != nil {
		// TODO
	}

	nextBatch := ""
	if !walker.Done() {
		cacheLine := walker.GetCached()
		nextBatch = paginationCache.AddLine(cacheLine)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: RoomHierarchyClientResponse{
			Rooms:     discoveredRooms,
			NextBatch: nextBatch,
		},
	}

}

type RoomHierarchyClientResponse struct {
	Rooms     []fclient.MSC2946Room `json:"rooms"`
	NextBatch string                `json:"next_batch,omitempty"`
}
