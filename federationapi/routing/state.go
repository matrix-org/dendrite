// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"net/http"
	"net/url"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetState returns state events & auth events for the roomID, eventID
func GetState(
	ctx context.Context,
	request *fclient.FederationRequest,
	rsAPI api.FederationRoomserverAPI,
	roomID string,
) util.JSONResponse {
	eventID, err := parseEventIDParam(request)
	if err != nil {
		return *err
	}

	stateEvents, authChain, err := getState(ctx, request, rsAPI, roomID, eventID)
	if err != nil {
		return *err
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: &fclient.RespState{
		AuthEvents:  types.NewEventJSONsFromHeaderedEvents(authChain),
		StateEvents: types.NewEventJSONsFromHeaderedEvents(stateEvents),
	}}
}

// GetStateIDs returns state event IDs & auth event IDs for the roomID, eventID
func GetStateIDs(
	ctx context.Context,
	request *fclient.FederationRequest,
	rsAPI api.FederationRoomserverAPI,
	roomID string,
) util.JSONResponse {
	eventID, err := parseEventIDParam(request)
	if err != nil {
		return *err
	}

	stateEvents, authEvents, err := getState(ctx, request, rsAPI, roomID, eventID)
	if err != nil {
		return *err
	}

	stateEventIDs := getIDsFromEvent(stateEvents)
	authEventIDs := getIDsFromEvent(authEvents)

	return util.JSONResponse{Code: http.StatusOK, JSON: fclient.RespStateIDs{
		StateEventIDs: stateEventIDs,
		AuthEventIDs:  authEventIDs,
	},
	}
}

func parseEventIDParam(
	request *fclient.FederationRequest,
) (eventID string, resErr *util.JSONResponse) {
	URL, err := url.Parse(request.RequestURI())
	if err != nil {
		response := util.ErrorResponse(err)
		resErr = &response
		return
	}

	eventID = URL.Query().Get("event_id")
	if eventID == "" {
		resErr = &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("event_id missing"),
		}
	}

	return
}

func getState(
	ctx context.Context,
	request *fclient.FederationRequest,
	rsAPI api.FederationRoomserverAPI,
	roomID string,
	eventID string,
) (stateEvents, authEvents []*types.HeaderedEvent, errRes *util.JSONResponse) {
	// If we don't think we belong to this room then don't waste the effort
	// responding to expensive requests for it.
	if err := ErrorIfLocalServerNotInRoom(ctx, rsAPI, roomID); err != nil {
		return nil, nil, err
	}

	event, resErr := fetchEvent(ctx, rsAPI, roomID, eventID)
	if resErr != nil {
		return nil, nil, resErr
	}

	if event.RoomID().String() != roomID {
		return nil, nil, &util.JSONResponse{Code: http.StatusNotFound, JSON: spec.NotFound("event does not belong to this room")}
	}
	resErr = allowedToSeeEvent(ctx, request.Origin(), rsAPI, eventID, event.RoomID().String())
	if resErr != nil {
		return nil, nil, resErr
	}

	var response api.QueryStateAndAuthChainResponse
	err := rsAPI.QueryStateAndAuthChain(
		ctx,
		&api.QueryStateAndAuthChainRequest{
			RoomID:       roomID,
			PrevEventIDs: []string{eventID},
			AuthEventIDs: event.AuthEventIDs(),
		},
		&response,
	)
	if err != nil {
		resErr := util.ErrorResponse(err)
		return nil, nil, &resErr
	}

	switch {
	case !response.RoomExists:
		return nil, nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Room not found"),
		}
	case !response.StateKnown:
		return nil, nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("State not known"),
		}
	case response.IsRejected:
		return nil, nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Event not found"),
		}
	}

	return response.StateEvents, response.AuthChainEvents, nil
}

func getIDsFromEvent(events []*types.HeaderedEvent) []string {
	IDs := make([]string, len(events))
	for i := range events {
		IDs[i] = events[i].EventID()
	}

	return IDs
}
