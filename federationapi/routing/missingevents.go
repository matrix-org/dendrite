// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type getMissingEventRequest struct {
	EarliestEvents []string `json:"earliest_events"`
	LatestEvents   []string `json:"latest_events"`
	Limit          int      `json:"limit"`
	MinDepth       int64    `json:"min_depth"` // not used
}

// GetMissingEvents returns missing events between earliest_events & latest_events.
// Events are fetched from room DAG starting from latest_events until we reach earliest_events or the limit.
func GetMissingEvents(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	rsAPI api.FederationRoomserverAPI,
	roomID string,
) util.JSONResponse {
	var gme getMissingEventRequest
	if err := json.Unmarshal(request.Content(), &gme); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	// If we don't think we belong to this room then don't waste the effort
	// responding to expensive requests for it.
	if err := ErrorIfLocalServerNotInRoom(httpReq.Context(), rsAPI, roomID); err != nil {
		return *err
	}

	var eventsResponse api.QueryMissingEventsResponse
	if err := rsAPI.QueryMissingEvents(
		httpReq.Context(), &api.QueryMissingEventsRequest{
			EarliestEvents: gme.EarliestEvents,
			LatestEvents:   gme.LatestEvents,
			Limit:          gme.Limit,
			ServerName:     request.Origin(),
		},
		&eventsResponse,
	); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("query.QueryMissingEvents failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	eventsResponse.Events = filterEvents(eventsResponse.Events, roomID)

	resp := fclient.RespMissingEvents{
		Events: types.NewEventJSONsFromHeaderedEvents(eventsResponse.Events),
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

// filterEvents returns only those events with matching roomID
func filterEvents(
	events []*types.HeaderedEvent, roomID string,
) []*types.HeaderedEvent {
	ref := events[:0]
	for _, ev := range events {
		if ev.RoomID().String() == roomID {
			ref = append(ref, ev)
		}
	}
	return ref
}
