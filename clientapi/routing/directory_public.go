// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"context"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/publicroomsapi/types"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
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
	req *http.Request, rsAPI roomserverAPI.RoomserverInternalAPI, stateAPI currentstateAPI.CurrentStateInternalAPI,
) util.JSONResponse {
	var request PublicRoomReq
	if fillErr := fillPublicRoomsReq(req, &request); fillErr != nil {
		return *fillErr
	}
	response, err := publicRooms(req.Context(), request, rsAPI, stateAPI)
	if err != nil {
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}

// GetPostPublicRoomsWithExternal is the same as GetPostPublicRooms but also mixes in public rooms from the provider supplied.
func GetPostPublicRoomsWithExternal(
	req *http.Request, rsAPI roomserverAPI.RoomserverInternalAPI, stateAPI currentstateAPI.CurrentStateInternalAPI,
	fedClient *gomatrixserverlib.FederationClient, extRoomsProvider types.ExternalPublicRoomsProvider,
) util.JSONResponse {
	var request PublicRoomReq
	if fillErr := fillPublicRoomsReq(req, &request); fillErr != nil {
		return *fillErr
	}
	response, err := publicRooms(req.Context(), request, rsAPI, stateAPI)
	if err != nil {
		return jsonerror.InternalServerError()
	}

	if request.Since != "" {
		// TODO: handle pagination tokens sensibly rather than ignoring them.
		// ignore paginated requests since we don't handle them yet over federation.
		// Only the initial request will contain federated rooms.
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: response,
		}
	}

	// If we have already hit the limit on the number of rooms, bail.
	var limit int
	if request.Limit > 0 {
		limit = int(request.Limit) - len(response.Chunk)
		if limit <= 0 {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: response,
			}
		}
	}

	// downcasting `limit` is safe as we know it isn't bigger than request.Limit which is int16
	fedRooms := bulkFetchPublicRoomsFromServers(req.Context(), fedClient, extRoomsProvider.Homeservers(), int16(limit))
	response.Chunk = append(response.Chunk, fedRooms...)

	// de-duplicate rooms with the same room ID. We can join the room via any of these aliases as we know these servers
	// are alive and well, so we arbitrarily pick one (purposefully shuffling them to spread the load a bit)
	var publicRooms []gomatrixserverlib.PublicRoom
	haveRoomIDs := make(map[string]bool)
	rand.Shuffle(len(response.Chunk), func(i, j int) {
		response.Chunk[i], response.Chunk[j] = response.Chunk[j], response.Chunk[i]
	})
	for _, r := range response.Chunk {
		if haveRoomIDs[r.RoomID] {
			continue
		}
		haveRoomIDs[r.RoomID] = true
		publicRooms = append(publicRooms, r)
	}
	// sort by member count
	sort.SliceStable(publicRooms, func(i, j int) bool {
		return publicRooms[i].JoinedMembersCount > publicRooms[j].JoinedMembersCount
	})

	response.Chunk = publicRooms

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}

// bulkFetchPublicRoomsFromServers fetches public rooms from the list of homeservers.
// Returns a list of public rooms up to the limit specified.
func bulkFetchPublicRoomsFromServers(
	ctx context.Context, fedClient *gomatrixserverlib.FederationClient, homeservers []string, limit int16,
) (publicRooms []gomatrixserverlib.PublicRoom) {
	// follow pipeline semantics, see https://blog.golang.org/pipelines for more info.
	// goroutines send rooms to this channel
	roomCh := make(chan gomatrixserverlib.PublicRoom, int(limit))
	// signalling channel to tell goroutines to stop sending rooms and quit
	done := make(chan bool)
	// signalling to say when we can close the room channel
	var wg sync.WaitGroup
	wg.Add(len(homeservers))
	// concurrently query for public rooms
	for _, hs := range homeservers {
		go func(homeserverDomain string) {
			defer wg.Done()
			util.GetLogger(ctx).WithField("hs", homeserverDomain).Info("Querying HS for public rooms")
			fres, err := fedClient.GetPublicRooms(ctx, gomatrixserverlib.ServerName(homeserverDomain), int(limit), "", false, "")
			if err != nil {
				util.GetLogger(ctx).WithError(err).WithField("hs", homeserverDomain).Warn(
					"bulkFetchPublicRoomsFromServers: failed to query hs",
				)
				return
			}
			for _, room := range fres.Chunk {
				// atomically send a room or stop
				select {
				case roomCh <- room:
				case <-done:
					util.GetLogger(ctx).WithError(err).WithField("hs", homeserverDomain).Info("Interrupted whilst sending rooms")
					return
				}
			}
		}(hs)
	}

	// Close the room channel when the goroutines have quit so we don't leak, but don't let it stop the in-flight request.
	// This also allows the request to fail fast if all HSes experience errors as it will cause the room channel to be
	// closed.
	go func() {
		wg.Wait()
		util.GetLogger(ctx).Info("Cleaning up resources")
		close(roomCh)
	}()

	// fan-in results with timeout. We stop when we reach the limit.
FanIn:
	for len(publicRooms) < int(limit) || limit == 0 {
		// add a room or timeout
		select {
		case room, ok := <-roomCh:
			if !ok {
				util.GetLogger(ctx).Info("All homeservers have been queried, returning results.")
				break FanIn
			}
			publicRooms = append(publicRooms, room)
		case <-time.After(15 * time.Second): // we've waited long enough, let's tell the client what we got.
			util.GetLogger(ctx).Info("Waited 15s for federated public rooms, returning early")
			break FanIn
		case <-ctx.Done(): // the client hung up on us, let's stop.
			util.GetLogger(ctx).Info("Client hung up, returning early")
			break FanIn
		}
	}
	// tell goroutines to stop
	close(done)

	return publicRooms
}

func publicRooms(ctx context.Context, request PublicRoomReq, rsAPI roomserverAPI.RoomserverInternalAPI,
	stateAPI currentstateAPI.CurrentStateInternalAPI) (*gomatrixserverlib.RespPublicRooms, error) {

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

	var queryRes roomserverAPI.QueryPublishedRoomsResponse
	err = rsAPI.QueryPublishedRooms(ctx, &roomserverAPI.QueryPublishedRoomsRequest{}, &queryRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryPublishedRooms failed")
		return nil, err
	}
	response.TotalRoomCountEstimate = len(queryRes.RoomIDs)

	if offset > 0 {
		response.PrevBatch = strconv.Itoa(int(offset) - 1)
	}
	nextIndex := int(offset) + int(limit)
	if response.TotalRoomCountEstimate > nextIndex {
		response.NextBatch = strconv.Itoa(nextIndex)
	}

	if offset < 0 {
		offset = 0
	}
	if nextIndex > len(queryRes.RoomIDs) {
		nextIndex = len(queryRes.RoomIDs)
	}
	roomIDs := queryRes.RoomIDs[offset:nextIndex]
	response.Chunk, err = fillInRooms(ctx, roomIDs, stateAPI)
	return &response, err
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

// due to lots of switches
// nolint:gocyclo
func fillInRooms(ctx context.Context, roomIDs []string, stateAPI currentstateAPI.CurrentStateInternalAPI) ([]gomatrixserverlib.PublicRoom, error) {
	avatarTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.avatar", StateKey: ""}
	nameTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.name", StateKey: ""}
	canonicalTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomCanonicalAlias, StateKey: ""}
	topicTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.topic", StateKey: ""}
	guestTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.guest_access", StateKey: ""}
	visibilityTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomHistoryVisibility, StateKey: ""}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomJoinRules, StateKey: ""}

	var stateRes currentstateAPI.QueryBulkStateContentResponse
	err := stateAPI.QueryBulkStateContent(ctx, &currentstateAPI.QueryBulkStateContentRequest{
		RoomIDs:        roomIDs,
		AllowWildcards: true,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			nameTuple, canonicalTuple, topicTuple, guestTuple, visibilityTuple, joinRuleTuple, avatarTuple,
			{EventType: gomatrixserverlib.MRoomMember, StateKey: "*"},
		},
	}, &stateRes)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("QueryBulkStateContent failed")
		return nil, err
	}
	chunk := make([]gomatrixserverlib.PublicRoom, len(roomIDs))
	i := 0
	for roomID, data := range stateRes.Rooms {
		pub := gomatrixserverlib.PublicRoom{
			RoomID: roomID,
		}
		joinCount := 0
		var joinRule, guestAccess string
		for tuple, contentVal := range data {
			if tuple.EventType == gomatrixserverlib.MRoomMember && contentVal == "join" {
				joinCount++
				continue
			}
			switch tuple {
			case avatarTuple:
				pub.AvatarURL = contentVal
			case nameTuple:
				pub.Name = contentVal
			case topicTuple:
				pub.Topic = contentVal
			case canonicalTuple:
				pub.CanonicalAlias = contentVal
			case visibilityTuple:
				pub.WorldReadable = contentVal == "world_readable"
			// need both of these to determine whether guests can join
			case joinRuleTuple:
				joinRule = contentVal
			case guestTuple:
				guestAccess = contentVal
			}
		}
		if joinRule == gomatrixserverlib.Public && guestAccess == "can_join" {
			pub.GuestCanJoin = true
		}
		pub.JoinedMembersCount = joinCount
		chunk[i] = pub
		i++
	}
	return chunk, nil
}
