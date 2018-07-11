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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// MakeLeave implements the /make_leave API
func MakeLeave(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg config.Dendrite,
	query api.RoomserverQueryAPI,
	roomID, userID string,
) util.JSONResponse {
	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Invalid UserID"),
		}
	}
	if domain != request.Origin() {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The leave must be sent by the server of the user"),
		}
	}

	// Try building an event for the server
	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     "m.room.member",
		StateKey: &userID,
	}
	err = builder.SetContent(map[string]interface{}{"membership": "leave"})
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	event, err := common.BuildEvent(httpReq.Context(), &builder, cfg, query, &queryRes)
	if err == common.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	} else if err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	// Check that the leave is allowed or not
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = &queryRes.StateEvents[i]
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)
	if err = gomatrixserverlib.Allowed(*event, &provider); err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{"event": builder},
	}
}

// SendLeave implements the /send_leave API
func SendLeave(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg config.Dendrite,
	producer *producers.RoomserverProducer,
	keys gomatrixserverlib.KeyRing,
	roomID, eventID string,
) util.JSONResponse {
	var event gomatrixserverlib.Event
	if err := json.Unmarshal(request.Content(), &event); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	// Check that the room ID is correct.
	if event.RoomID() != roomID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The room ID in the request path must match the room ID in the leave event JSON"),
		}
	}

	// Check that the event ID is correct.
	if event.EventID() != eventID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event ID in the request path must match the event ID in the leave event JSON"),
		}
	}

	// Check that the event is from the server sending the request.
	if event.Origin() != request.Origin() {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The leave must be sent by the server it originated on"),
		}
	}

	// Check that the event is signed by the server sending the request.
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName: event.Origin(),
		Message:    event.Redact().JSON(),
		AtTS:       event.OriginServerTS(),
	}}
	verifyResults, err := keys.VerifyJSONs(httpReq.Context(), verifyRequests)
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	}
	if verifyResults[0].Error != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The leave must be signed by the server it originated on"),
		}
	}

	// check membership is set to leave
	mem, err := event.Membership()
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	} else if mem != "leave" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The membership in the event content must be set to leave"),
		}
	}

	// Send the events to the room server.
	// We are responsible for notifying other servers that the user has left
	// the room, so set SendAsServer to cfg.Matrix.ServerName
	_, err = producer.SendEvents(httpReq.Context(), []gomatrixserverlib.Event{event}, cfg.Matrix.ServerName, nil)
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
