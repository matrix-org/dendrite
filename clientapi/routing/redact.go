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
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/common/transactions"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// https://matrix.org/docs/spec/client_server/r0.5.0#put-matrix-client-r0-rooms-roomid-redact-eventid-txnid

type redactRequest struct {
	Reason string `json:"reason,omitempty"`
}

type redactResponse struct {
	EventID string `json:"event_id"`
}

// Redact implements PUT /_matrix/client/r0/rooms/{roomId}/redact/{eventId}/{txnId}
// and POST /_matrix/client/r0/rooms/{roomId}/redact/{eventId} (mainly for SyTest)
func Redact(
	req *http.Request,
	device *authtypes.Device,
	roomID, redactedEventID string, txnID *string,
	cfg config.Dendrite,
	queryAPI api.RoomserverQueryAPI,
	producer *producers.RoomserverProducer,
	txnCache *transactions.Cache,
) util.JSONResponse {
	// TODO: Idempotency

	var redactReq redactRequest
	if resErr := httputil.UnmarshalJSONRequest(req, &redactReq); resErr != nil {
		return *resErr
	}

	// Build a redaction event
	builder := gomatrixserverlib.EventBuilder{
		Sender:  device.UserID,
		RoomID:  roomID,
		Redacts: redactedEventID,
		Type:    gomatrixserverlib.MRoomRedaction,
	}
	err := builder.SetContent(redactReq)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	e, err := common.BuildEvent(req.Context(), &builder, cfg, time.Now(), queryAPI, &queryRes)
	if err == common.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	} else if err != nil {
		return httputil.LogThenError(req, err)
	}

	if resErr := checkRedactionAllowed(
		req, queryAPI, e, queryRes.StateEvents,
	); resErr != nil {
		return *resErr
	}

	// Send the redaction event

	var txnAndDeviceID *api.TransactionID
	if txnID != nil {
		txnAndDeviceID = &api.TransactionID{
			TransactionID: *txnID,
			DeviceID:      device.ID,
		}
	}

	// pass the new event to the roomserver and receive the correct event ID
	// event ID in case of duplicate transaction is discarded
	eventID, err := producer.SendEvents(
		req.Context(), []gomatrixserverlib.Event{*e}, cfg.Matrix.ServerName, txnAndDeviceID,
	)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: redactResponse{eventID},
	}
}

func checkRedactionAllowed(
	req *http.Request, queryAPI api.RoomserverQueryAPI,
	redactionEvent *gomatrixserverlib.Event,
	stateEvents []gomatrixserverlib.Event,
) *util.JSONResponse {
	// Do some basic checks e.g. ensuring the user is in the room and can send m.room.redaction events
	statEventPointers := make([]*gomatrixserverlib.Event, len(stateEvents))
	for i := range stateEvents {
		statEventPointers[i] = &stateEvents[i]
	}
	provider := gomatrixserverlib.NewAuthEvents(statEventPointers)
	if err := gomatrixserverlib.Allowed(*redactionEvent, &provider); err != nil {
		// TODO: Is the error returned with suitable HTTP status code?
		if _, ok := err.(*gomatrixserverlib.NotAllowed); ok {
			return &util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden(err.Error()),
			}
		}

		res := httputil.LogThenError(req, err)
		return &res
	}

	// Ensure the user can redact the specific event

	eventReq := api.QueryEventsByIDRequest{
		EventIDs: []string{redactionEvent.Redacts()},
	}
	var eventResp api.QueryEventsByIDResponse
	if err := queryAPI.QueryEventsByID(req.Context(), &eventReq, &eventResp); err != nil {
		res := httputil.LogThenError(req, err)
		return &res
	}

	if len(eventResp.Events) == 0 {
		return &util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Event to redact not found"),
		}
	}

	redactedEvent := eventResp.Events[0]

	badEvents, _, err := common.ValidateRedaction(&redactedEvent, redactionEvent)
	if err != nil {
		res := httputil.LogThenError(req, err)
		return &res
	}
	if badEvents {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("invalid redaction attempt"),
		}
	}

	if redactedEvent.Sender() != redactionEvent.Sender() {
		// TODO: Allow power users to redact others' events
		return &util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("You are not allowed to redact this event"),
		}
	}

	return nil
}
