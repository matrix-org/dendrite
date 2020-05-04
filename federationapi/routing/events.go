// Copyright 2017 New Vector Ltd
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
	"encoding/json"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetEvent returns the requested event
func GetEvent(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	rsAPI api.RoomserverInternalAPI,
	eventID string,
	origin gomatrixserverlib.ServerName,
) util.JSONResponse {
	event, err := getEvent(ctx, request, rsAPI, eventID)
	if err != nil {
		return *err
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: gomatrixserverlib.Transaction{
		Origin:         origin,
		OriginServerTS: gomatrixserverlib.AsTimestamp(time.Now()),
		PDUs: []json.RawMessage{
			event.JSON(),
		},
	}}
}

// getEvent returns the requested event,
// otherwise it returns an error response which can be sent to the client.
func getEvent(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	rsAPI api.RoomserverInternalAPI,
	eventID string,
) (*gomatrixserverlib.Event, *util.JSONResponse) {
	var authResponse api.QueryServerAllowedToSeeEventResponse
	err := rsAPI.QueryServerAllowedToSeeEvent(
		ctx,
		&api.QueryServerAllowedToSeeEventRequest{
			EventID:    eventID,
			ServerName: request.Origin(),
		},
		&authResponse,
	)
	if err != nil {
		resErr := util.ErrorResponse(err)
		return nil, &resErr
	}

	if !authResponse.AllowedToSeeEvent {
		resErr := util.MessageResponse(http.StatusForbidden, "server not allowed to see event")
		return nil, &resErr
	}

	var eventsResponse api.QueryEventsByIDResponse
	err = rsAPI.QueryEventsByID(
		ctx,
		&api.QueryEventsByIDRequest{EventIDs: []string{eventID}},
		&eventsResponse,
	)
	if err != nil {
		resErr := util.ErrorResponse(err)
		return nil, &resErr
	}

	if len(eventsResponse.Events) == 0 {
		return nil, &util.JSONResponse{Code: http.StatusNotFound, JSON: nil}
	}

	return &eventsResponse.Events[0].Event, nil
}
