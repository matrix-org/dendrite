// Copyright 2018-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// Backfill implements the /backfill federation endpoint.
// https://matrix.org/docs/spec/server_server/unstable.html#get-matrix-federation-v1-backfill-roomid
func Backfill(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	rsAPI api.FederationRoomserverAPI,
	roomID string,
	cfg *config.FederationAPI,
) util.JSONResponse {
	var res api.PerformBackfillResponse
	var eIDs []string
	var limit string
	var exists bool
	var err error

	// Check the room ID's format.
	if _, _, err = gomatrixserverlib.SplitID('!', roomID); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("Bad room ID: " + err.Error()),
		}
	}

	// If we don't think we belong to this room then don't waste the effort
	// responding to expensive requests for it.
	if err := ErrorIfLocalServerNotInRoom(httpReq.Context(), rsAPI, roomID); err != nil {
		return *err
	}

	// Check if all of the required parameters are there.
	eIDs, exists = httpReq.URL.Query()["v"]
	if !exists {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("v is missing"),
		}
	}
	limit = httpReq.URL.Query().Get("limit")
	if len(limit) == 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("limit is missing"),
		}
	}

	// Populate the request.
	req := api.PerformBackfillRequest{
		RoomID: roomID,
		// we don't know who the successors are for these events, which won't
		// be a problem because we don't use that information when servicing /backfill requests,
		// only when making them. TODO: Think of a better API shape
		BackwardsExtremities: map[string][]string{
			"": eIDs,
		},
		ServerName:  request.Origin(),
		VirtualHost: request.Destination(),
	}
	if req.Limit, err = strconv.Atoi(limit); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("strconv.Atoi failed")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(fmt.Sprintf("limit %q is invalid format", limit)),
		}
	}

	// Enforce a limit of 100 events, as not to hit the DB to hard.
	// Synapse has a hard limit of 100 events as well.
	if req.Limit > 100 {
		req.Limit = 100
	}

	// Query the Roomserver.
	if err = rsAPI.PerformBackfill(httpReq.Context(), &req, &res); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("query.PerformBackfill failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Filter any event that's not from the requested room out.
	evs := make([]gomatrixserverlib.PDU, 0)

	var ev *types.HeaderedEvent
	for _, ev = range res.Events {
		if ev.RoomID().String() == roomID {
			evs = append(evs, ev.PDU)
		}
	}

	eventJSONs := []json.RawMessage{}
	for _, e := range gomatrixserverlib.ReverseTopologicalOrdering(
		evs,
		gomatrixserverlib.TopologicalOrderByPrevEvents,
	) {
		eventJSONs = append(eventJSONs, e.JSON())
	}

	// sytest wants these in reversed order, similar to /messages, so reverse them now.
	for i := len(eventJSONs)/2 - 1; i >= 0; i-- {
		opp := len(eventJSONs) - 1 - i
		eventJSONs[i], eventJSONs[opp] = eventJSONs[opp], eventJSONs[i]
	}

	txn := gomatrixserverlib.Transaction{
		Origin:         request.Destination(),
		PDUs:           eventJSONs,
		OriginServerTS: spec.AsTimestamp(time.Now()),
	}

	// Send the events to the client.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: txn,
	}
}
