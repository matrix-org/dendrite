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
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// MakeLeave implements the /make_leave API
func MakeLeave(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
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
	err = builder.SetContent(map[string]interface{}{"membership": gomatrixserverlib.Leave})
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("builder.SetContent failed")
		return jsonerror.InternalServerError()
	}

	identity, err := cfg.Matrix.SigningIdentityFor(request.Destination())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(
				fmt.Sprintf("Server name %q does not exist", request.Destination()),
			),
		}
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	event, err := eventutil.QueryAndBuildEvent(httpReq.Context(), &builder, cfg.Matrix, identity, time.Now(), rsAPI, &queryRes)
	if err == eventutil.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	} else if e, ok := err.(gomatrixserverlib.BadJSONError); ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(e.Error()),
		}
	} else if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
		return jsonerror.InternalServerError()
	}

	// If the user has already left then just return their last leave
	// event. This means that /send_leave will be a no-op, which helps
	// to reject invites multiple times - hopefully.
	for _, state := range queryRes.StateEvents {
		if !state.StateKeyEquals(userID) {
			continue
		}
		if mem, merr := state.Membership(); merr == nil && mem == gomatrixserverlib.Leave {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: map[string]interface{}{
					"room_version": event.RoomVersion,
					"event":        state,
				},
			}
		}
	}

	// Check that the leave is allowed or not
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = queryRes.StateEvents[i].Event
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)
	if err = gomatrixserverlib.Allowed(event.Event, &provider); err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			"room_version": event.RoomVersion,
			"event":        builder,
		},
	}
}

// SendLeave implements the /send_leave API
// nolint:gocyclo
func SendLeave(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
	roomID, eventID string,
) util.JSONResponse {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), &verReq, &verRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion(err.Error()),
		}
	}

	// Decode the event JSON from the request.
	event, err := gomatrixserverlib.NewEventFromUntrustedJSON(request.Content(), verRes.RoomVersion)
	switch err.(type) {
	case gomatrixserverlib.BadJSONError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	case nil:
	default:
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

	if event.StateKey() == nil || event.StateKeyEquals("") {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("No state key was provided in the leave event."),
		}
	}
	if !event.StateKeyEquals(event.Sender()) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Event state key must match the event sender."),
		}
	}

	// Check that the sender belongs to the server that is sending us
	// the request. By this point we've already asserted that the sender
	// and the state key are equal so we don't need to check both.
	var serverName gomatrixserverlib.ServerName
	if _, serverName, err = gomatrixserverlib.SplitID('@', event.Sender()); err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The sender of the join is invalid"),
		}
	} else if serverName != request.Origin() {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The sender does not match the server that originated the request"),
		}
	}

	// Check if the user has already left. If so, no-op!
	queryReq := &api.QueryLatestEventsAndStateRequest{
		RoomID: roomID,
		StateToFetch: []gomatrixserverlib.StateKeyTuple{
			{
				EventType: gomatrixserverlib.MRoomMember,
				StateKey:  *event.StateKey(),
			},
		},
	}
	queryRes := &api.QueryLatestEventsAndStateResponse{}
	err = rsAPI.QueryLatestEventsAndState(httpReq.Context(), queryReq, queryRes)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryLatestEventsAndState failed")
		return jsonerror.InternalServerError()
	}
	// The room doesn't exist or we weren't ever joined to it. Might as well
	// no-op here.
	if !queryRes.RoomExists || len(queryRes.StateEvents) == 0 {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	// Check if we're recycling a previous leave event.
	if event.EventID() == queryRes.StateEvents[0].EventID() {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	// We are/were joined/invited/banned or something. Check if
	// we can no-op here.
	if len(queryRes.StateEvents) == 1 {
		if mem, merr := queryRes.StateEvents[0].Membership(); merr == nil && mem == gomatrixserverlib.Leave {
			return util.JSONResponse{
				Code: http.StatusOK,
				JSON: struct{}{},
			}
		}
	}

	// Check that the event is signed by the server sending the request.
	redacted, err := gomatrixserverlib.RedactEventJSON(event.JSON(), event.Version())
	if err != nil {
		logrus.WithError(err).Errorf("XXX: leave.go")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event JSON could not be redacted"),
		}
	}
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName:             serverName,
		Message:                redacted,
		AtTS:                   event.OriginServerTS(),
		StrictValidityChecking: true,
	}}
	verifyResults, err := keys.VerifyJSONs(httpReq.Context(), verifyRequests)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("keys.VerifyJSONs failed")
		return jsonerror.InternalServerError()
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
		util.GetLogger(httpReq.Context()).WithError(err).Error("event.Membership failed")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing content.membership key"),
		}
	}
	if mem != gomatrixserverlib.Leave {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The membership in the event content must be set to leave"),
		}
	}

	// Send the events to the room server.
	// We are responsible for notifying other servers that the user has left
	// the room, so set SendAsServer to cfg.Matrix.ServerName
	var response api.InputRoomEventsResponse
	if err := rsAPI.InputRoomEvents(httpReq.Context(), &api.InputRoomEventsRequest{
		InputRoomEvents: []api.InputRoomEvent{
			{
				Kind:          api.KindNew,
				Event:         event.Headered(verRes.RoomVersion),
				SendAsServer:  string(cfg.Matrix.ServerName),
				TransactionID: nil,
			},
		},
	}, &response); err != nil {
		return jsonerror.InternalAPIError(httpReq.Context(), err)
	}

	if response.ErrMsg != "" {
		util.GetLogger(httpReq.Context()).WithField(logrus.ErrorKey, response.ErrMsg).WithField("not_allowed", response.NotAllowed).Error("producer.SendEvents failed")
		if response.NotAllowed {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.Forbidden(response.ErrMsg),
			}
		}
		return jsonerror.InternalServerError()
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
