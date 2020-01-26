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

package canonical_alias

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/util"
)

// RoomserverCanonicalAliasAPIDatabase has the storage APIs needed to implement the alias API.
type RoomserverCanonicalAPIDatabase interface {
	// Save a given canonical room alias with the room ID it refers to.
	// Returns an error if there was a problem talking to the database.
	SetRoomCanonicalAlias(ctx context.Context, canonical_alias string, roomID string, creatorUserID string) error
	// Look up the room ID a given canonical alias refers to.
	// Returns an error if there was a problem talking to the database.
	GetRoomIDForCanonicalAlias(ctx context.Context, canonical_alias string) (string, error)
	// Look up the canonical alias referring to a given room ID.
	// Returns an error if there was a problem talking to the database.
	GetCanonicalAliasForRoomID(ctx context.Context, roomID string) (string, error)
	// Get the user ID of the creator of the canonical alias.
	// Returns an error if there was a problem talking to the database.
	GetCreatorIDForCanonicalAlias(ctx context.Context, canonical_alias string) (string, error)
	// Remove a given room canonical alias.
	// Returns an error if there was a problem talking to the database.
	RemoveRoomCanonicalAlias(ctx context.Context, roomID string) error
}

// RoomserverCanonicalAliasAPI is an implementation of alias.RoomserverCanonicalAliasAPI
type RoomserverCanonicalAliasAPI struct {
	DB       RoomserverCanonicalAPIDatabase
	Cfg      *config.Dendrite
	AliasAPI roomserverAPI.RoomserverAliasAPI
}

// SetRoomCanonicalAlias implements alias.RoomserverCanonicalAliasAPI
func (r *RoomserverCanonicalAliasAPI) SetRoomCanonicalAlias(
	ctx context.Context,
	request *roomserverAPI.SetRoomCanonicalAliasRequest,
	response *roomserverAPI.SetRoomCanonicalAliasResponse,
) error {
	// SPEC: Room with `m.room.canonical_alias` with empty alias field should be
	// treated same as room without a canonical alias.
	if request.CanonicalAlias == "" {
		return r.DB.RemoveRoomCanonicalAlias(ctx, request.RoomID)
	}

	aliasReq := roomserverAPI.GetRoomIDForAliasRequest{Alias: request.CanonicalAlias}
	var aliasResp roomserverAPI.GetRoomIDForAliasResponse
	err := r.AliasAPI.GetRoomIDForAlias(ctx, &aliasReq, &aliasResp)
	if err != nil {
		return err
	}

	// Check if the alias has been assigned to a different room
	if len(aliasResp.RoomID) > 0 && aliasResp.RoomID != request.RoomID {
		response.CanonicalAliasExists = true
		return nil
	}

	// Create alias if not already assigned
	if aliasResp.RoomID == "" {
		// RFC: Creating an alias if not already is convienent for users
		// but makes the code coupled with changes in alias.go
		setAliasReq := roomserverAPI.SetRoomAliasRequest{
			Alias:  request.CanonicalAlias,
			RoomID: request.RoomID,
			UserID: request.UserID,
		}
		var setAliasResp roomserverAPI.SetRoomAliasResponse
		err = r.AliasAPI.SetRoomAlias(ctx, &setAliasReq, &setAliasResp)
		if err != nil {
			return err
		}
	}

	// Save the new canonical alias
	err = r.DB.SetRoomCanonicalAlias(ctx, request.CanonicalAlias, request.RoomID, request.UserID)
	if err != nil {
		return err
	}

	return nil
}

// GetRoomIDForCanonicalAlias implements alias.RoomserverCanonicalAliasAPI
func (r *RoomserverCanonicalAliasAPI) GetRoomIDForCanonicalAlias(
	ctx context.Context,
	request *roomserverAPI.GetRoomIDForCanonicalAliasRequest,
	response *roomserverAPI.GetRoomIDForCanonicalAliasResponse,
) error {
	// Look up the room ID in the database
	roomID, err := r.DB.GetRoomIDForCanonicalAlias(ctx, request.CanonicalAlias)
	if err != nil {
		return err
	}

	// RFC: Should we search in application service for canonical aliases?`
	response.RoomID = roomID
	return nil
}

// GetCanonicalAliasForRoomID implements alias.RoomserverCanonicalAliasAPI
func (r *RoomserverCanonicalAliasAPI) GetCanonicalAliasForRoomID(
	ctx context.Context,
	request *roomserverAPI.GetCanonicalAliasForRoomIDRequest,
	response *roomserverAPI.GetCanonicalAliasForRoomIDResponse,
) error {
	// Look up the canonical alias in the database for the given RoomID
	canonicalAlias, err := r.DB.GetCanonicalAliasForRoomID(ctx, request.RoomID)
	if err != nil {
		return err
	}

	response.CanonicalAlias = canonicalAlias
	return nil
}

// GetCreatorIDForCanonicalAlias implements alias.RoomserverCanonicalAliasAPI
func (r *RoomserverCanonicalAliasAPI) GetCreatorIDForCanonicalAlias(
	ctx context.Context,
	request *roomserverAPI.GetCreatorIDForCanonicalAliasRequest,
	response *roomserverAPI.GetCreatorIDForCanonicalAliasResponse,
) error {
	// Look up the creator id in the database for the given CanonicalAlias
	creatorID, err := r.DB.GetCreatorIDForCanonicalAlias(ctx, request.CanonicalAlias)
	if err != nil {
		return err
	}

	response.UserID = creatorID
	return nil
}

// SetupHTTP adds the RoomserverCanonicalAliasAPI handlers to the http.ServeMux.
func (r *RoomserverCanonicalAliasAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		roomserverAPI.RoomserverSetRoomCanonicalAliasPath,
		common.MakeInternalAPI("setRoomCanonicalAlias", func(req *http.Request) util.JSONResponse {
			var request roomserverAPI.SetRoomCanonicalAliasRequest
			var response roomserverAPI.SetRoomCanonicalAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.SetRoomCanonicalAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		roomserverAPI.RoomserverGetRoomIDForCanonicalAliasPath,
		common.MakeInternalAPI("GetRoomIDForCanonicalAlias", func(req *http.Request) util.JSONResponse {
			var request roomserverAPI.GetRoomIDForCanonicalAliasRequest
			var response roomserverAPI.GetRoomIDForCanonicalAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetRoomIDForCanonicalAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		roomserverAPI.RoomserverGetCreatorIDForCanonicalAliasPath,
		common.MakeInternalAPI("GetCreatorIDForCanonicalAlias", func(req *http.Request) util.JSONResponse {
			var request roomserverAPI.GetCreatorIDForCanonicalAliasRequest
			var response roomserverAPI.GetCreatorIDForCanonicalAliasResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetCreatorIDForCanonicalAlias(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(
		roomserverAPI.RoomserverGetCanonicalAliasForRoomIDPath,
		common.MakeInternalAPI("getCanonicalAliasForRoomID", func(req *http.Request) util.JSONResponse {
			var request roomserverAPI.GetCanonicalAliasForRoomIDRequest
			var response roomserverAPI.GetCanonicalAliasForRoomIDResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.GetCanonicalAliasForRoomID(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
