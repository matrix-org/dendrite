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

package readers

import (
	"context"
	"time"

	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetEvent returns the requested event
func GetEvent(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	cfg config.Dendrite,
	query api.RoomserverQueryAPI,
	now time.Time,
	keys gomatrixserverlib.KeyRing,
	eventID string,
) util.JSONResponse {
	var authResponse api.QueryServerAllowedToSeeEventResponse
	err := query.QueryServerAllowedToSeeEvent(
		ctx,
		&api.QueryServerAllowedToSeeEventRequest{
			EventID:    eventID,
			ServerName: request.Origin(),
		},
		&authResponse,
	)
	if err != nil {
		return util.ErrorResponse(err)
	}

	if !authResponse.AllowedToSeeEvent {
		return util.MessageResponse(403, "server not allowed to see event")
	}

	var eventsResponse api.QueryEventsByIDResponse
	err = query.QueryEventsByID(
		ctx,
		&api.QueryEventsByIDRequest{EventIDs: []string{eventID}},
		&eventsResponse,
	)
	if err != nil {
		return util.ErrorResponse(err)
	}

	if len(eventsResponse.Events) == 0 {
		return util.JSONResponse{Code: 404, JSON: nil}
	}

	return util.JSONResponse{Code: 200, JSON: &eventsResponse.Events[0]}
}
