// Copyright 2019 Alex Chen
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
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type getEventRequest struct {
	req            *http.Request
	device         *authtypes.Device
	roomID         string
	eventID        string
	cfg            config.Dendrite
	federation     *gomatrixserverlib.FederationClient
	keyRing        gomatrixserverlib.KeyRing
	requestedEvent gomatrixserverlib.Event
}

// GetEvent implements GET /_matrix/client/r0/rooms/{roomId}/event/{eventId}
// https://matrix.org/docs/spec/client_server/r0.4.0.html#get-matrix-client-r0-rooms-roomid-event-eventid
func GetEvent(
	req *http.Request,
	device *authtypes.Device,
	roomID string,
	eventID string,
	cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
	federation *gomatrixserverlib.FederationClient,
	keyRing gomatrixserverlib.KeyRing,
) util.JSONResponse {
	eventsReq := api.QueryEventsByIDRequest{
		EventIDs: []string{eventID},
	}
	var eventsResp api.QueryEventsByIDResponse
	err := queryAPI.QueryEventsByID(req.Context(), &eventsReq, &eventsResp)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	var requestedEvent gomatrixserverlib.Event
	if len(eventsResp.Events) == 0 {
		// Event not found locally. Do a federation query in hope of getting
		// the event from another server.
		// TODO: May need a better way to determine which server to query
		_, domain, err := gomatrixserverlib.SplitID('!', roomID)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		txnResp, err := federation.GetEvent(req.Context(), domain, eventID)
		if err != nil {
			return httputil.LogThenError(req, err)
		}

		if len(txnResp.PDUs) == 0 {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound("The event was not found or you do not have permission to read this event."),
			}
		}

		requestedEvent = txnResp.PDUs[0]
	} else {
		requestedEvent = eventsResp.Events[0]
	}

	r := getEventRequest{
		req:            req,
		device:         device,
		roomID:         roomID,
		eventID:        eventID,
		cfg:            cfg,
		federation:     federation,
		keyRing:        keyRing,
		requestedEvent: requestedEvent,
	}

	stateNeeded := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{r.requestedEvent})
	stateReq := api.QueryStateAfterEventsRequest{
		RoomID:       r.requestedEvent.RoomID(),
		PrevEventIDs: r.requestedEvent.PrevEventIDs(),
		StateToFetch: stateNeeded.Tuples(),
	}
	var stateResp api.QueryStateAfterEventsResponse
	if err := queryAPI.QueryStateAfterEvents(req.Context(), &stateReq, &stateResp); err != nil {
		return httputil.LogThenError(req, err)
	}

	if !stateResp.RoomExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("The event was not found or you do not have permission to read this event."),
		}
	}

	if !stateResp.PrevEventsExist {
		// Missing some events locally so stateResp.StateEvents will be unavailable.
		// Do a federation query in hope of getting the state events needed.
		return r.proceedWithMissingState()
	}

	return r.proceedWithStateEvents(stateResp.StateEvents)
}

// proceedWithMissingState tries to proceed by fetching the missing state over
// federation.
// Note: It's not guaranteed that the server(s) we query have the state events.
func (r *getEventRequest) proceedWithMissingState() util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('!', r.roomID)
	if err != nil {
		return httputil.LogThenError(r.req, err)
	}

	if domain == r.cfg.Matrix.ServerName {
		// Don't send a federation query to self
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("The event was not found or you do not have permission to read this event."),
		}
	}

	state, err := r.federation.LookupState(r.req.Context(), domain, r.roomID, r.eventID)
	if err != nil {
		return httputil.LogThenError(r.req, err)
	}

	if err := state.Check(r.req.Context(), r.keyRing); err != nil {
		return httputil.LogThenError(r.req, err)
	}

	return r.proceedWithStateEvents(state.StateEvents)
}

func (r *getEventRequest) proceedWithStateEvents(stateEvents []gomatrixserverlib.Event) util.JSONResponse {
	allowed := false
	for _, stateEvent := range stateEvents {
		if stateEvent.StateKeyEquals(r.device.UserID) {
			membership, err := stateEvent.Membership()
			if err == nil && membership == "join" {
				allowed = true
				break
			}
		}
	}

	if allowed {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: r.requestedEvent,
		}
	}

	return util.JSONResponse{
		Code: http.StatusNotFound,
		JSON: jsonerror.NotFound("The event was not found or you do not have permission to read this event."),
	}
}
