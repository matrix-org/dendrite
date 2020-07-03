// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type redactionContent struct {
	Reason string `json:"reason"`
}

type redactionResponse struct {
	EventID string `json:"event_id"`
}

func SendRedaction(
	req *http.Request, device *userapi.Device, roomID, eventID string, cfg *config.Dendrite,
	rsAPI roomserverAPI.RoomserverInternalAPI, stateAPI currentstateAPI.CurrentStateInternalAPI,
) util.JSONResponse {
	resErr := checkMemberInRoom(req.Context(), stateAPI, device.UserID, roomID)
	if resErr != nil {
		return *resErr
	}

	ev := roomserverAPI.GetEvent(req.Context(), rsAPI, eventID)
	if ev == nil {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.NotFound("unknown event ID"), // TODO: is it ok to leak existence?
		}
	}
	if ev.RoomID() != roomID {
		return util.JSONResponse{
			Code: 400,
			JSON: jsonerror.NotFound("cannot redact event in another room"),
		}
	}

	// "Users may redact their own events, and any user with a power level greater than or equal
	// to the redact power level of the room may redact events there"
	// https://matrix.org/docs/spec/client_server/r0.6.1#put-matrix-client-r0-rooms-roomid-redact-eventid-txnid
	allowedToRedact := ev.Sender() == device.UserID
	if !allowedToRedact {
		plEvent := currentstateAPI.GetEvent(req.Context(), stateAPI, roomID, gomatrixserverlib.StateKeyTuple{
			EventType: gomatrixserverlib.MRoomPowerLevels,
			StateKey:  "",
		})
		if plEvent == nil {
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.Forbidden("cannot redact event, missing pl event"),
			}
		}
		pl, err := plEvent.PowerLevels()
		if err != nil {
			return util.JSONResponse{
				Code: 403,
				JSON: jsonerror.Forbidden("cannot redact event, malformed pl event"),
			}
		}
		allowedToRedact = pl.UserLevel(device.UserID) >= pl.Redact
	}
	if !allowedToRedact {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden("cannot redact event"),
		}
	}

	var r redactionContent
	resErr = httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	// create the new event and set all the fields we can
	builder := gomatrixserverlib.EventBuilder{
		Sender:  device.UserID,
		RoomID:  roomID,
		Type:    gomatrixserverlib.MRoomRedaction,
		Redacts: eventID,
	}
	err := builder.SetContent(r)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("builder.SetContent failed")
		return jsonerror.InternalServerError()
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	e, err := eventutil.BuildEvent(req.Context(), &builder, cfg, time.Now(), rsAPI, &queryRes)
	if err == eventutil.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	}
	_, err = roomserverAPI.SendEvents(context.Background(), rsAPI, []gomatrixserverlib.HeaderedEvent{*e}, cfg.Matrix.ServerName, nil)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Errorf("failed to SendEvents")
		return jsonerror.InternalServerError()
	}
	return util.JSONResponse{
		Code: 200,
		JSON: redactionResponse{
			EventID: e.EventID(),
		},
	}
}
