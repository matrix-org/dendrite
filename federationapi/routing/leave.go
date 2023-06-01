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

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// MakeLeave implements the /make_leave API
func MakeLeave(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	roomID spec.RoomID, userID spec.UserID,
) util.JSONResponse {
	roomVersion, err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), roomID.String())
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("failed obtaining room version")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	req := api.QueryServerJoinedToRoomRequest{
		ServerName: request.Destination(),
		RoomID:     roomID.String(),
	}
	res := api.QueryServerJoinedToRoomResponse{}
	if err := rsAPI.QueryServerJoinedToRoom(httpReq.Context(), &req, &res); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryServerJoinedToRoom failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	createLeaveTemplate := func(proto *gomatrixserverlib.ProtoEvent) (gomatrixserverlib.PDU, []gomatrixserverlib.PDU, error) {
		identity, err := cfg.Matrix.SigningIdentityFor(request.Destination())
		if err != nil {
			util.GetLogger(httpReq.Context()).WithError(err).Errorf("obtaining signing identity for %s failed", request.Destination())
			return nil, nil, spec.NotFound(fmt.Sprintf("Server name %q does not exist", request.Destination()))
		}

		queryRes := api.QueryLatestEventsAndStateResponse{}
		event, err := eventutil.QueryAndBuildEvent(httpReq.Context(), proto, identity, time.Now(), rsAPI, &queryRes)
		switch e := err.(type) {
		case nil:
		case eventutil.ErrRoomNoExists:
			util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.NotFound("Room does not exist")
		case gomatrixserverlib.BadJSONError:
			util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.BadJSON(e.Error())
		default:
			util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.InternalServerError{}
		}

		stateEvents := make([]gomatrixserverlib.PDU, len(queryRes.StateEvents))
		for i, stateEvent := range queryRes.StateEvents {
			stateEvents[i] = stateEvent.PDU
		}
		return event, stateEvents, nil
	}

	input := gomatrixserverlib.HandleMakeLeaveInput{
		UserID:             userID,
		RoomID:             roomID,
		RoomVersion:        roomVersion,
		RequestOrigin:      request.Origin(),
		LocalServerName:    cfg.Matrix.ServerName,
		LocalServerInRoom:  res.RoomExists && res.IsInRoom,
		BuildEventTemplate: createLeaveTemplate,
	}

	response, internalErr := gomatrixserverlib.HandleMakeLeave(input)
	switch e := internalErr.(type) {
	case nil:
	case spec.InternalServerError:
		util.GetLogger(httpReq.Context()).WithError(internalErr).Error("failed to handle make_leave request")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	case spec.MatrixError:
		util.GetLogger(httpReq.Context()).WithError(internalErr).Error("failed to handle make_leave request")
		code := http.StatusInternalServerError
		switch e.ErrCode {
		case spec.ErrorForbidden:
			code = http.StatusForbidden
		case spec.ErrorNotFound:
			code = http.StatusNotFound
		case spec.ErrorBadJSON:
			code = http.StatusBadRequest
		}

		return util.JSONResponse{
			Code: code,
			JSON: e,
		}
	default:
		util.GetLogger(httpReq.Context()).WithError(internalErr).Error("failed to handle make_leave request")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("unknown error"),
		}
	}

	if response == nil {
		util.GetLogger(httpReq.Context()).Error("gmsl.HandleMakeLeave returned invalid response")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			"event":        response.LeaveTemplateEvent,
			"room_version": response.RoomVersion,
		},
	}
}

// SendLeave implements the /send_leave API
// nolint:gocyclo
func SendLeave(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
	roomID, eventID string,
) util.JSONResponse {
	roomVersion, err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.UnsupportedRoomVersion(err.Error()),
		}
	}

	event, err := gomatrixserverlib.HandleSendLeave(
		httpReq.Context(), request.Content(), request.Origin(), roomVersion, eventID, roomID, rsAPI, keys,
	)

	switch e := err.(type) {
	case nil:
	case spec.InternalServerError:
		util.GetLogger(httpReq.Context()).WithError(err).Error("failed to handle send_leave request")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	case spec.MatrixError:
		util.GetLogger(httpReq.Context()).WithError(err).Error("failed to handle send_leave request")
		code := http.StatusInternalServerError
		switch e.ErrCode {
		case spec.ErrorForbidden:
			code = http.StatusForbidden
		case spec.ErrorNotFound:
			code = http.StatusNotFound
		case spec.ErrorUnsupportedRoomVersion:
			code = http.StatusInternalServerError
		case spec.ErrorBadJSON:
			code = http.StatusBadRequest
		}

		return util.JSONResponse{
			Code: code,
			JSON: e,
		}
	default:
		util.GetLogger(httpReq.Context()).WithError(err).Error("failed to handle send_leave request")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("unknown error"),
		}
	}

	// Send the events to the room server.
	// We are responsible for notifying other servers that the user has left
	// the room, so set SendAsServer to cfg.Matrix.ServerName
	var response api.InputRoomEventsResponse
	rsAPI.InputRoomEvents(httpReq.Context(), &api.InputRoomEventsRequest{
		InputRoomEvents: []api.InputRoomEvent{
			{
				Kind:          api.KindNew,
				Event:         &types.HeaderedEvent{PDU: event},
				SendAsServer:  string(cfg.Matrix.ServerName),
				TransactionID: nil,
			},
		},
	}, &response)

	if response.ErrMsg != "" {
		util.GetLogger(httpReq.Context()).WithField(logrus.ErrorKey, response.ErrMsg).WithField("not_allowed", response.NotAllowed).Error("producer.SendEvents failed")
		if response.NotAllowed {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.Forbidden(response.ErrMsg),
			}
		}
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
