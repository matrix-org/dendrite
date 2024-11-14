// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	"github.com/element-hq/dendrite/clientapi/api"
	"github.com/element-hq/dendrite/clientapi/httputil"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/config"
)

var (
	cacheMu          sync.Mutex
	publicRoomsCache []fclient.PublicRoom
)

type PublicRoomReq struct {
	Since              string `json:"since,omitempty"`
	Limit              int64  `json:"limit,omitempty"`
	Filter             filter `json:"filter,omitempty"`
	Server             string `json:"server,omitempty"`
	IncludeAllNetworks bool   `json:"include_all_networks,omitempty"`
	NetworkID          string `json:"third_party_instance_id,omitempty"`
}

type filter struct {
	SearchTerms string   `json:"generic_search_term,omitempty"`
	RoomTypes   []string `json:"room_types,omitempty"` // TODO: Implement filter on this
}

// GetPostPublicRooms implements GET and POST /publicRooms
func GetPostPublicRooms(
	req *http.Request, rsAPI roomserverAPI.ClientRoomserverAPI,
	extRoomsProvider api.ExtraPublicRoomsProvider,
	federation fclient.FederationClient,
	cfg *config.ClientAPI,
) util.JSONResponse {
	var request PublicRoomReq
	if fillErr := fillPublicRoomsReq(req, &request); fillErr != nil {
		return *fillErr
	}

	if request.IncludeAllNetworks && request.NetworkID != "" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("include_all_networks and third_party_instance_id can not be used together"),
		}
	}

	serverName := spec.ServerName(request.Server)
	if serverName != "" && !cfg.Matrix.IsLocalServerName(serverName) {
		res, err := federation.GetPublicRoomsFiltered(
			req.Context(), cfg.Matrix.ServerName, serverName,
			int(request.Limit), request.Since,
			request.Filter.SearchTerms, false,
			"",
		)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("failed to get public rooms")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: res,
		}
	}

	response, err := publicRooms(req.Context(), request, rsAPI, extRoomsProvider)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Errorf("failed to work out public rooms")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}

func publicRooms(
	ctx context.Context, request PublicRoomReq, rsAPI roomserverAPI.ClientRoomserverAPI, extRoomsProvider api.ExtraPublicRoomsProvider,
) (*fclient.RespPublicRooms, error) {

	response := fclient.RespPublicRooms{
		Chunk: []fclient.PublicRoom{},
	}
	var limit int64
	var offset int64
	limit = request.Limit
	if limit == 0 {
		limit = 50
	}
	offset, err := strconv.ParseInt(request.Since, 10, 64)
	// ParseInt returns 0 and an error when trying to parse an empty string
	// In that case, we want to assign 0 so we ignore the error
	if err != nil && len(request.Since) > 0 {
		util.GetLogger(ctx).WithError(err).Error("strconv.ParseInt failed")
		return nil, err
	}
	err = nil

	var rooms []fclient.PublicRoom
	if request.Since == "" {
		rooms = refreshPublicRoomCache(ctx, rsAPI, extRoomsProvider, request)
	} else {
		rooms = getPublicRoomsFromCache()
	}

	response.TotalRoomCountEstimate = len(rooms)

	rooms = filterRooms(rooms, request.Filter.SearchTerms)

	chunk, prev, next := sliceInto(rooms, offset, limit)
	if prev >= 0 {
		response.PrevBatch = "T" + strconv.Itoa(prev)
	}
	if next >= 0 {
		response.NextBatch = "T" + strconv.Itoa(next)
	}
	if chunk != nil {
		response.Chunk = chunk
	}
	return &response, err
}

func filterRooms(rooms []fclient.PublicRoom, searchTerm string) []fclient.PublicRoom {
	if searchTerm == "" {
		return rooms
	}

	normalizedTerm := strings.ToLower(searchTerm)

	result := make([]fclient.PublicRoom, 0)
	for _, room := range rooms {
		if strings.Contains(strings.ToLower(room.Name), normalizedTerm) ||
			strings.Contains(strings.ToLower(room.Topic), normalizedTerm) ||
			strings.Contains(strings.ToLower(room.CanonicalAlias), normalizedTerm) {
			result = append(result, room)
		}
	}

	return result
}

// fillPublicRoomsReq fills the Limit, Since and Filter attributes of a GET or POST request
// on /publicRooms by parsing the incoming HTTP request
// Filter is only filled for POST requests
func fillPublicRoomsReq(httpReq *http.Request, request *PublicRoomReq) *util.JSONResponse {
	if httpReq.Method != "GET" && httpReq.Method != "POST" {
		return &util.JSONResponse{
			Code: http.StatusMethodNotAllowed,
			JSON: spec.NotFound("Bad method"),
		}
	}
	if httpReq.Method == "GET" {
		limit, err := strconv.Atoi(httpReq.FormValue("limit"))
		// Atoi returns 0 and an error when trying to parse an empty string
		// In that case, we want to assign 0 so we ignore the error
		if err != nil && len(httpReq.FormValue("limit")) > 0 {
			util.GetLogger(httpReq.Context()).WithError(err).Error("strconv.Atoi failed")
			return &util.JSONResponse{
				Code: 400,
				JSON: spec.BadJSON("limit param is not a number"),
			}
		}
		request.Limit = int64(limit)
		request.Since = httpReq.FormValue("since")
		request.Server = httpReq.FormValue("server")
	} else {
		resErr := httputil.UnmarshalJSONRequest(httpReq, request)
		if resErr != nil {
			return resErr
		}
		request.Server = httpReq.FormValue("server")
	}

	// strip the 'T' which is only required because when sytest does pagination tests it stops
	// iterating when !prev_batch which then fails if prev_batch==0, so add arbitrary text to
	// make it truthy not falsey.
	request.Since = strings.TrimPrefix(request.Since, "T")
	return nil
}

// sliceInto returns a subslice of `slice` which honours the since/limit values given.
//
//	  0  1  2  3  4  5  6   index
//	 [A, B, C, D, E, F, G]  slice
//
//	 limit=3          => A,B,C (prev='', next='3')
//	 limit=3&since=3  => D,E,F (prev='0', next='6')
//	 limit=3&since=6  => G     (prev='3', next='')
//
//	A value of '-1' for prev/next indicates no position.
func sliceInto(slice []fclient.PublicRoom, since int64, limit int64) (subset []fclient.PublicRoom, prev, next int) {
	prev = -1
	next = -1

	if since > 0 {
		prev = int(since) - int(limit)
	}
	nextIndex := int(since) + int(limit)
	if len(slice) > nextIndex { // there are more rooms ahead of us
		next = nextIndex
	}

	// apply sanity caps
	if since < 0 {
		since = 0
	}
	if nextIndex > len(slice) {
		nextIndex = len(slice)
	}

	subset = slice[since:nextIndex]
	return
}

func refreshPublicRoomCache(
	ctx context.Context, rsAPI roomserverAPI.ClientRoomserverAPI, extRoomsProvider api.ExtraPublicRoomsProvider,
	request PublicRoomReq,
) []fclient.PublicRoom {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	var extraRooms []fclient.PublicRoom
	if extRoomsProvider != nil {
		extraRooms = extRoomsProvider.Rooms()
	}

	// TODO: this is only here to make Sytest happy, for now.
	ns := strings.Split(request.NetworkID, "|")
	if len(ns) == 2 {
		request.NetworkID = ns[1]
	}

	var queryRes roomserverAPI.QueryPublishedRoomsResponse
	err := rsAPI.QueryPublishedRooms(ctx, &roomserverAPI.QueryPublishedRoomsRequest{
		NetworkID:          request.NetworkID,
		IncludeAllNetworks: request.IncludeAllNetworks,
	}, &queryRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryPublishedRooms failed")
		return publicRoomsCache
	}
	pubRooms, err := roomserverAPI.PopulatePublicRooms(ctx, queryRes.RoomIDs, rsAPI)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("PopulatePublicRooms failed")
		return publicRoomsCache
	}
	publicRoomsCache = []fclient.PublicRoom{}
	publicRoomsCache = append(publicRoomsCache, pubRooms...)
	publicRoomsCache = append(publicRoomsCache, extraRooms...)
	publicRoomsCache = dedupeAndShuffle(publicRoomsCache)

	// sort by total joined member count (big to small)
	sort.SliceStable(publicRoomsCache, func(i, j int) bool {
		return publicRoomsCache[i].JoinedMembersCount > publicRoomsCache[j].JoinedMembersCount
	})
	return publicRoomsCache
}

func getPublicRoomsFromCache() []fclient.PublicRoom {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	return publicRoomsCache
}

func dedupeAndShuffle(in []fclient.PublicRoom) []fclient.PublicRoom {
	// de-duplicate rooms with the same room ID. We can join the room via any of these aliases as we know these servers
	// are alive and well, so we arbitrarily pick one (purposefully shuffling them to spread the load a bit)
	var publicRooms []fclient.PublicRoom
	haveRoomIDs := make(map[string]bool)
	rand.Shuffle(len(in), func(i, j int) {
		in[i], in[j] = in[j], in[i]
	})
	for _, r := range in {
		if haveRoomIDs[r.RoomID] {
			continue
		}
		haveRoomIDs[r.RoomID] = true
		publicRooms = append(publicRooms, r)
	}
	return publicRooms
}
