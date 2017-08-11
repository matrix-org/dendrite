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

package publicroom

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

// RoomserverPublicRoomAPIDatabase has the storage APIs needed to implement the
// public room API.
type RoomserverPublicRoomAPIDatabase interface {
	// Lookup the numeric ID for the room.
	// Returns 0 if the room doesn't exists.
	// Returns an error if there was a problem talking to the database.
	RoomNID(roomID string) (types.RoomNID, error)
	// Lookup the room IDs matching a batch of room numeric IDs, ordered by
	// numeric ID (ie map[roomNID] = roomID).
	// Returns an error if the retrieval failed.
	RoomIDs([]types.RoomNID) (map[types.RoomNID]string, error)
	// Checks the visibility of the room identified by the given numeric ID.
	// Returns true if the room is publicly visible, returns false if not.
	// If there's no room matching this numeric ID, or if the retrieval failed,
	// returns an error.
	IsRoomPublic(roomNID types.RoomNID) (bool, error)
	// Returns an array of string containing the room numeric IDs of the rooms
	// that are publicly visible.
	// Returns an error if the retrieval failed.
	GetPublicRoomNIDs() ([]types.RoomNID, error)
	// Updates the visibility for a room to the given value: true means that the
	// room is publicly visible, false means that the room isn't publicly visible.
	// Returns an error if the update failed.
	UpdateRoomVisibility(roomNID types.RoomNID, visibility bool) error
	// Returns a map of the aliases bound to a given set of room numeric IDs,
	// ordered by room NID (ie map[roomNID] = []alias)
	// Returns an error if the retrieval failed
	GetAliasesFromRoomNIDs(roomNIDs []types.RoomNID) (map[types.RoomNID][]string, error)
}

// RoomserverPublicRoomAPI is an implementation of api.RoomserverPublicRoomAPI
type RoomserverPublicRoomAPI struct {
	DB RoomserverPublicRoomAPIDatabase
}

// SetRoomVisibility implements api.RoomserverPublicRoomAPI
func (r *RoomserverPublicRoomAPI) SetRoomVisibility(
	req *api.SetRoomVisibilityRequest,
	response *api.SetRoomVisibilityResponse,
) error {
	roomNID, err := r.DB.RoomNID(req.RoomID)
	if err != nil || roomNID == 0 {
		return err
	}

	var visibility bool
	if req.Visibility == "public" {
		visibility = true
	} else if req.Visibility == "private" {
		visibility = false
	} else {
		return errors.New("Invalid visibility setting")
	}

	if err = r.DB.UpdateRoomVisibility(roomNID, visibility); err != nil {
		return err
	}

	return nil
}

// GetRoomVisibility implements api.RoomserverPublicRoomAPI
func (r *RoomserverPublicRoomAPI) GetRoomVisibility(
	req *api.GetRoomVisibilityRequest,
	response *api.GetRoomVisibilityResponse,
) error {
	roomNID, err := r.DB.RoomNID(req.RoomID)
	if err != nil || roomNID == 0 {
		return err
	}

	if isPublic, err := r.DB.IsRoomPublic(roomNID); err != nil {
		return err
	} else if isPublic {
		response.Visibility = "public"
	} else {
		response.Visibility = "private"
	}

	return nil
}

// GetPublicRooms implements api.RoomserverPublicRoomAPI
func (r *RoomserverPublicRoomAPI) GetPublicRooms(
	req *api.GetPublicRoomsRequest,
	response *api.GetPublicRoomsResponse,
) error {
	var limit int16
	var offset int64

	limit = req.Limit
	offset, err := strconv.ParseInt(req.Since, 10, 64)
	// ParseInt returns 0 and an error when trying to parse an empty string
	// In that case, we want to assign 0 so we ignore the error
	if err != nil && len(req.Since) > 0 {
		return err
	}

	roomNIDs, err := r.DB.GetPublicRoomNIDs()
	if err != nil {
		return err
	}

	aliases, err := r.DB.GetAliasesFromRoomNIDs(roomNIDs)
	if err != nil {
		return err
	}
	roomIDs, err := r.DB.RoomIDs(roomNIDs)
	if err != nil {
		return err
	}

	chunks := []api.PublicRoomsChunk{}
	// Iterate over the array of aliases instead of the array of rooms, because
	// a room must have at least one alias to be listed
	for roomNID, as := range aliases {
		chunk := api.PublicRoomsChunk{
			RoomID:           roomIDs[roomNID],
			Aliases:          as,
			NumJoinedMembers: 0,
			WorldReadable:    true,
			GuestCanJoin:     true,
		}
		chunks = append(chunks, chunk)
	}

	chunks = chunks[offset:]

	if len(chunks) >= int(limit) {
		chunks = chunks[offset:limit]
	}

	response.Chunks = chunks

	return nil
}

// SetupHTTP adds the RoomserverPublicRoomAPI handlers to the http.ServeMux.
func (r *RoomserverPublicRoomAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.RoomserverSetRoomVisibilityPath,
		common.MakeAPI("setRoomVisibility", func(req *http.Request) util.JSONResponse {
			var request api.SetRoomVisibilityRequest
			var response api.SetRoomVisibilityResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.SetRoomVisibility(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverGetRoomVisibilityPath,
		common.MakeAPI("getRoomVisibility", func(req *http.Request) util.JSONResponse {
			var request api.GetRoomVisibilityRequest
			var response api.GetRoomVisibilityResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetRoomVisibility(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverGetPublicRoomsPath,
		common.MakeAPI("getPublicRooms", func(req *http.Request) util.JSONResponse {
			var request api.GetPublicRoomsRequest
			var response api.GetPublicRoomsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetPublicRooms(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
}
