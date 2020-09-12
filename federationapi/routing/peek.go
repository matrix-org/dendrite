// Copyright 2020 New Vector Ltd
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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// Peek implements the /peek API
func Peek(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.RoomserverInternalAPI,
	roomID, peekID string,
	remoteVersions []gomatrixserverlib.RoomVersion,
) util.JSONResponse {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), &verReq, &verRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.InternalServerError(),
		}
	}

	// Check that the room that the remote side is trying to join is actually
	// one of the room versions that they listed in their supported ?ver= in
	// the peek URL.
	remoteSupportsVersion := false
	for _, v := range remoteVersions {
		if v == verRes.RoomVersion {
			remoteSupportsVersion = true
			break
		}
	}
	// If it isn't, stop trying to join the room.
	if !remoteSupportsVersion {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.IncompatibleRoomVersion(verRes.RoomVersion),
		}
	}

	// TODO: Check history visibility

	state, err := getCurrentState(httpReq.Context(), request, rsAPI, roomID)
	if err != nil {
		return *err
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			"state":        state,
			"room_version": verRes.RoomVersion,
		},
	}
}


func getCurrentState(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	rsAPI api.RoomserverInternalAPI,
	roomID string,
) (*gomatrixserverlib.RespPeek, *util.JSONResponse) {
	var response api.QueryStateAndAuthChainResponse
	err := rsAPI.QueryStateAndAuthChain(
		ctx,
		&api.QueryStateAndAuthChainRequest{
			RoomID:       roomID,
			PrevEventIDs: []string{},
			AuthEventIDs: []string{},
		},
		&response,
	)
	if err != nil {
		resErr := util.ErrorResponse(err)
		return nil, &resErr
	}

	if !response.RoomExists {
		return nil, &util.JSONResponse{Code: http.StatusNotFound, JSON: nil}
	}

	return &gomatrixserverlib.RespPeek{
		StateEvents: gomatrixserverlib.UnwrapEventHeaders(response.StateEvents),
		AuthEvents:  gomatrixserverlib.UnwrapEventHeaders(response.AuthChainEvents),
		RoomVersion: response.RoomVersion,
		RenewalInterval: 60 * 60 * 1000 * 1000, // one hour
	}, nil
}
