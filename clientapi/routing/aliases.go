// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/util"
)

// GetAliases implements GET /_matrix/client/r0/rooms/{roomId}/aliases
func GetAliases(
	req *http.Request, rsAPI api.ClientRoomserverAPI, device *userapi.Device, roomID string,
) util.JSONResponse {
	stateTuple := gomatrixserverlib.StateKeyTuple{
		EventType: gomatrixserverlib.MRoomHistoryVisibility,
		StateKey:  "",
	}
	stateReq := &api.QueryCurrentStateRequest{
		RoomID:      roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{stateTuple},
	}
	stateRes := &api.QueryCurrentStateResponse{}
	if err := rsAPI.QueryCurrentState(req.Context(), stateReq, stateRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryCurrentState failed")
		return util.ErrorResponse(fmt.Errorf("rsAPI.QueryCurrentState: %w", err))
	}

	visibility := gomatrixserverlib.HistoryVisibilityInvited
	if historyVisEvent, ok := stateRes.StateEvents[stateTuple]; ok {
		var err error
		visibility, err = historyVisEvent.HistoryVisibility()
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("historyVisEvent.HistoryVisibility failed")
			return util.ErrorResponse(fmt.Errorf("historyVisEvent.HistoryVisibility: %w", err))
		}
	}
	if visibility != gomatrixserverlib.WorldReadable {
		queryReq := api.QueryMembershipForUserRequest{
			RoomID: roomID,
			UserID: device.UserID,
		}
		var queryRes api.QueryMembershipForUserResponse
		if err := rsAPI.QueryMembershipForUser(req.Context(), &queryReq, &queryRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryMembershipsForRoom failed")
			return jsonerror.InternalServerError()
		}
		if !queryRes.IsInRoom {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden("You aren't a member of this room."),
			}
		}
	}

	aliasesReq := api.GetAliasesForRoomIDRequest{
		RoomID: roomID,
	}
	aliasesRes := api.GetAliasesForRoomIDResponse{}
	if err := rsAPI.GetAliasesForRoomID(req.Context(), &aliasesReq, &aliasesRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.GetAliasesForRoomID failed")
		return util.ErrorResponse(fmt.Errorf("rsAPI.GetAliasesForRoomID: %w", err))
	}

	response := struct {
		Aliases []string `json:"aliases"`
	}{
		Aliases: aliasesRes.Aliases,
	}
	if response.Aliases == nil {
		response.Aliases = []string{} // pleases sytest
	}

	return util.JSONResponse{
		Code: 200,
		JSON: response,
	}
}
