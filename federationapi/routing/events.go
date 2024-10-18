// Copyright 2017-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// GetEvent returns the requested event
func GetEvent(
	ctx context.Context,
	request *fclient.FederationRequest,
	rsAPI api.FederationRoomserverAPI,
	eventID string,
	origin spec.ServerName,
) util.JSONResponse {
	// /_matrix/federation/v1/event/{eventId} doesn't have a roomID, we use an empty string,
	// which results in `QueryEventsByID` to first get the event and use that to determine the roomID.
	event, err := fetchEvent(ctx, rsAPI, "", eventID)
	if err != nil {
		return *err
	}

	err = allowedToSeeEvent(ctx, request.Origin(), rsAPI, eventID, event.RoomID().String())
	if err != nil {
		return *err
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: gomatrixserverlib.Transaction{
		Origin:         origin,
		OriginServerTS: spec.AsTimestamp(time.Now()),
		PDUs: []json.RawMessage{
			event.JSON(),
		},
	}}
}

// allowedToSeeEvent returns no error if the server is allowed to see this event,
// otherwise it returns an error response which can be sent to the client.
func allowedToSeeEvent(
	ctx context.Context,
	origin spec.ServerName,
	rsAPI api.FederationRoomserverAPI,
	eventID string,
	roomID string,
) *util.JSONResponse {
	allowed, err := rsAPI.QueryServerAllowedToSeeEvent(ctx, origin, eventID, roomID)
	if err != nil {
		resErr := util.ErrorResponse(err)
		return &resErr
	}

	if !allowed {
		resErr := util.MessageResponse(http.StatusForbidden, "server not allowed to see event")
		return &resErr
	}

	return nil
}

// fetchEvent fetches the event without auth checks. Returns an error if the event cannot be found.
func fetchEvent(ctx context.Context, rsAPI api.FederationRoomserverAPI, roomID, eventID string) (gomatrixserverlib.PDU, *util.JSONResponse) {
	var eventsResponse api.QueryEventsByIDResponse
	err := rsAPI.QueryEventsByID(
		ctx,
		&api.QueryEventsByIDRequest{EventIDs: []string{eventID}, RoomID: roomID},
		&eventsResponse,
	)
	if err != nil {
		resErr := util.ErrorResponse(err)
		return nil, &resErr
	}

	if len(eventsResponse.Events) == 0 {
		return nil, &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Event not found"),
		}
	}

	return eventsResponse.Events[0].PDU, nil
}
