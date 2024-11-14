// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"net/http"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetEventAuth returns event auth for the roomID and eventID
func GetEventAuth(
	ctx context.Context,
	request *fclient.FederationRequest,
	rsAPI api.FederationRoomserverAPI,
	roomID string,
	eventID string,
) util.JSONResponse {
	// If we don't think we belong to this room then don't waste the effort
	// responding to expensive requests for it.
	if err := ErrorIfLocalServerNotInRoom(ctx, rsAPI, roomID); err != nil {
		return *err
	}

	event, resErr := fetchEvent(ctx, rsAPI, roomID, eventID)
	if resErr != nil {
		return *resErr
	}

	if event.RoomID().String() != roomID {
		return util.JSONResponse{Code: http.StatusNotFound, JSON: spec.NotFound("event does not belong to this room")}
	}
	resErr = allowedToSeeEvent(ctx, request.Origin(), rsAPI, eventID, event.RoomID().String())
	if resErr != nil {
		return *resErr
	}

	var response api.QueryStateAndAuthChainResponse
	err := rsAPI.QueryStateAndAuthChain(
		ctx,
		&api.QueryStateAndAuthChainRequest{
			RoomID:             roomID,
			PrevEventIDs:       []string{eventID},
			AuthEventIDs:       event.AuthEventIDs(),
			OnlyFetchAuthChain: true,
		},
		&response,
	)
	if err != nil {
		return util.ErrorResponse(err)
	}

	if !response.RoomExists {
		return util.JSONResponse{Code: http.StatusNotFound, JSON: nil}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: fclient.RespEventAuth{
			AuthEvents: types.NewEventJSONsFromHeaderedEvents(response.AuthChainEvents),
		},
	}
}
