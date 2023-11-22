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
	log "github.com/sirupsen/logrus"
)

// For storing pagination information for room hierarchies
type RoomHierarchyPaginationCache struct {
	cache map[string]roomserverAPI.RoomHierarchyWalker
	mu    sync.Mutex
}

// Create a new, empty, pagination cache.
func NewRoomHierarchyPaginationCache() RoomHierarchyPaginationCache {
	return RoomHierarchyPaginationCache{
		cache: map[string]roomserverAPI.RoomHierarchyWalker{},
	}
}

// Get a cached page, or nil if there is no associated page in the cache.
func (c *RoomHierarchyPaginationCache) Get(token string) *roomserverAPI.RoomHierarchyWalker {
	c.mu.Lock()
	defer c.mu.Unlock()
	line, ok := c.cache[token]
	if ok {
		return &line
	} else {
		return nil
	}
}

// Add a cache line to the pagination cache.
func (c *RoomHierarchyPaginationCache) AddLine(line roomserverAPI.RoomHierarchyWalker) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	token := uuid.NewString()
	c.cache[token] = line
	return token
}

// Query the hierarchy of a room/space
//
// Implements /_matrix/client/v1/rooms/{roomID}/hierarchy
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
		var maybeLimit int
		maybeLimit, err = strconv.Atoi(limitStr)
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
		var maybeMaxDepth int
		maybeMaxDepth, err = strconv.Atoi(maxDepthStr)
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
		walker = roomserverAPI.NewRoomHierarchyWalker(types.NewDeviceNotServerName(*device), roomID, suggestedOnly, maxDepth)
	} else { // Attempt to resume cached walker
		cachedWalker := paginationCache.Get(from)

		if cachedWalker == nil || cachedWalker.SuggestedOnly != suggestedOnly || cachedWalker.MaxDepth != maxDepth {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("pagination not found for provided token ('from') with given 'max_depth', 'suggested_only' and room ID"),
			}
		}

		walker = *cachedWalker
	}

	discoveredRooms, nextWalker, err := rsAPI.QueryNextRoomHierarchyPage(req.Context(), walker, limit)

	if err != nil {
		switch err.(type) {
		case roomserverAPI.ErrRoomUnknownOrNotAllowed:
			util.GetLogger(req.Context()).WithError(err).Debugln("room unknown/forbidden when handling CS room hierarchy request")
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden("room is unknown/forbidden"),
			}
		default:
			log.WithError(err).Errorf("failed to fetch next page of room hierarchy (CS API)")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown("internal server error"),
			}
		}
	}

	nextBatch := ""
	// nextWalker will be nil if there's no more rooms left to walk
	if nextWalker != nil {
		nextBatch = paginationCache.AddLine(*nextWalker)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: RoomHierarchyClientResponse{
			Rooms:     discoveredRooms,
			NextBatch: nextBatch,
		},
	}

}

// Success response for /_matrix/client/v1/rooms/{roomID}/hierarchy
type RoomHierarchyClientResponse struct {
	Rooms     []fclient.RoomHierarchyRoom `json:"rooms"`
	NextBatch string                      `json:"next_batch,omitempty"`
}
