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
	"sync"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/publicroomsapi/storage"
	"github.com/matrix-org/dendrite/publicroomsapi/types"
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
	response, err := publicRooms(req.Context(), request, publicRoomDatabase)
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
	req *http.Request, publicRoomDatabase storage.Database, fedClient *gomatrixserverlib.FederationClient,
	extRoomsProvider types.ExternalPublicRoomsProvider,
) util.JSONResponse {
	var request PublicRoomReq
	if fillErr := fillPublicRoomsReq(req, &request); fillErr != nil {
		return *fillErr
	}
	response, err := publicRooms(req.Context(), request, publicRoomDatabase)
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
					util.GetLogger(ctx).WithError(err).WithField("hs", homeserverDomain).Info("Interruped whilst sending rooms")
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

func publicRooms(ctx context.Context, request PublicRoomReq, publicRoomDatabase storage.Database) (*gomatrixserverlib.RespPublicRooms, error) {
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
