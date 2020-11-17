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

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetState returns state events & auth events for the roomID, eventID
func GetState(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	rsAPI api.RoomserverInternalAPI,
	roomID string,
) util.JSONResponse {
	eventID, err := parseEventIDParam(request)
	if err != nil {
		return *err
	}

	state, err := getState(ctx, request, rsAPI, roomID, eventID)
	if err != nil {
		return *err
	}

	return util.JSONResponse{Code: http.StatusOK, JSON: state}
}

// GetStateIDs returns state event IDs & auth event IDs for the roomID, eventID
func GetStateIDs(
	ctx context.Context,
	request *gomatrixserverlib.FederationRequest,
	rsAPI api.RoomserverInternalAPI,
	roomID string,
) util.JSONResponse {
	eventID, err := parseEventIDParam(request)
	if err != nil {
		return *err
	}

	state, err := getState(ctx, request, rsAPI, roomID, eventID)
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
		response := util.ErrorResponse(err)
		resErr = &response
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
	rsAPI api.RoomserverInternalAPI,
	roomID string,
	eventID string,
) (*gomatrixserverlib.RespState, *util.JSONResponse) {
	event, resErr := fetchEvent(ctx, rsAPI, eventID)
	if resErr != nil {
		return nil, resErr
	}

	if event.RoomID() != roomID {
		return nil, &util.JSONResponse{Code: http.StatusNotFound, JSON: jsonerror.NotFound("event does not belong to this room")}
	}
	resErr = allowedToSeeEvent(ctx, request.Origin(), rsAPI, eventID)
	if resErr != nil {
		return nil, resErr
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
		return nil, &resErr
	}

	if !response.RoomExists {
		return nil, &util.JSONResponse{Code: http.StatusNotFound, JSON: nil}
	}

	return &gomatrixserverlib.RespState{
		StateEvents: gomatrixserverlib.UnwrapEventHeaders(response.StateEvents),
		AuthEvents:  gomatrixserverlib.UnwrapEventHeaders(response.AuthChainEvents),
	}, nil
}

func getIDsFromEvent(events []*gomatrixserverlib.Event) []string {
	IDs := make([]string, len(events))
	for i := range events {
		IDs[i] = events[i].EventID()
	}

	return IDs
}
