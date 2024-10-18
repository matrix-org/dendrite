// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/element-hq/dendrite/roomserver/api"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetAliases implements GET /_matrix/client/r0/rooms/{roomId}/aliases
func GetAliases(
	req *http.Request, rsAPI api.ClientRoomserverAPI, device *userapi.Device, roomID string,
) util.JSONResponse {
	stateTuple := gomatrixserverlib.StateKeyTuple{
		EventType: spec.MRoomHistoryVisibility,
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
		var content gomatrixserverlib.HistoryVisibilityContent
		if err = json.Unmarshal(historyVisEvent.Content(), &content); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("historyVisEvent.HistoryVisibility failed")
			return util.ErrorResponse(fmt.Errorf("historyVisEvent.HistoryVisibility: %w", err))
		}
		visibility = content.HistoryVisibility
	}
	if visibility != spec.WorldReadable {
		deviceUserID, err := spec.NewUserID(device.UserID, true)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden("userID doesn't have power level to change visibility"),
			}
		}
		queryReq := api.QueryMembershipForUserRequest{
			RoomID: roomID,
			UserID: *deviceUserID,
		}
		var queryRes api.QueryMembershipForUserResponse
		if err := rsAPI.QueryMembershipForUser(req.Context(), &queryReq, &queryRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryMembershipsForRoom failed")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		if !queryRes.IsInRoom {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: spec.Forbidden("You aren't a member of this room."),
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
