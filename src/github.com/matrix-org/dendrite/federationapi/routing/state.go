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
	"net/url"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetState returns state events & auth events for the roomID, eventID
func GetState(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	_ config.Dendrite,
	query api.RoomserverQueryAPI,
	_ time.Time,
	_ gomatrixserverlib.KeyRing,
	roomID string,
) util.JSONResponse {
	eventID, err := parseEventIDParam(request)
	if err != nil {
		return *err
	}

	state, err := getState(ctx, request, query, roomID, eventID)
	if err != nil {
		return *err
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: state}
}

// GetStateIDs returns state event IDs & auth event IDs for the roomID, eventID
func GetStateIDs(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	_ config.Dendrite,
	query api.RoomserverQueryAPI,
	_ time.Time,
	_ gomatrixserverlib.KeyRing,
	roomID string,
) util.JSONResponse {
	eventID, err := parseEventIDParam(request)
	if err != nil {
		return *err
	}

	state, err := getState(ctx, request, query, roomID, eventID)
	if err != nil {
		return *err
	}

	stateEventIDs := getIDsFromEvent(state.StateEvents)
	authEventIDs := getIDsFromEvent(state.AuthEvents)

	return util.JSONResponse{Code: http.StatusOK, JSON: gomatrixserverlib.RespStateIDs{
		StateEventIDs: stateEventIDs,
		AuthEventIDs:  authEventIDs,
	},
	}
}

func parseEventIDParam(
	request *gomatrixserverlib.FederationRequest,
) (eventID string, resErr *util.JSONResponse) {
	URL, err := url.Parse(request.RequestURI())
	if err != nil {
		*resErr = util.ErrorResponse(err)
		return
	}

	eventID = URL.Query().Get("event_id")
	if eventID == "" {
		resErr = &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("event_id missing"),
		}
	}

	return
}

func getState(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	query api.RoomserverQueryAPI,
	roomID string,
	eventID string,
) (*gomatrixserverlib.RespState, *util.JSONResponse) {
	event, resErr := getEvent(ctx, request, query, eventID)
	if resErr != nil {
		return nil, resErr
	}

	prevEventIDs := getIDsFromEventRef(event.PrevEvents())
	authEventIDs := getIDsFromEventRef(event.AuthEvents())

	var response api.QueryStateAndAuthChainResponse
	err := query.QueryStateAndAuthChain(
		ctx,
		&api.QueryStateAndAuthChainRequest{
			RoomID:       roomID,
			PrevEventIDs: prevEventIDs,
			AuthEventIDs: authEventIDs,
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

	return &gomatrixserverlib.RespState{
		StateEvents: response.StateEvents,
		AuthEvents:  response.AuthChainEvents,
	}, nil
}

func getIDsFromEventRef(events []gomatrixserverlib.EventReference) []string {
	IDs := make([]string, len(events))
	for i := range events {
		IDs[i] = events[i].EventID
	}

	return IDs
}

func getIDsFromEvent(events []gomatrixserverlib.Event) []string {
	IDs := make([]string, len(events))
	for i := range events {
		IDs[i] = events[i].EventID()
	}

	return IDs
}
