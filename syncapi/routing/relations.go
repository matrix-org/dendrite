// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"
	"strconv"

	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/api"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/internal"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type RelationsResponse struct {
	Chunk     []synctypes.ClientEvent `json:"chunk"`
	NextBatch string                  `json:"next_batch,omitempty"`
	PrevBatch string                  `json:"prev_batch,omitempty"`
}

// nolint:gocyclo
func Relations(
	req *http.Request, device *userapi.Device,
	syncDB storage.Database,
	rsAPI api.SyncRoomserverAPI,
	rawRoomID, eventID, relType, eventType string,
) util.JSONResponse {
	roomID, err := spec.NewRoomID(rawRoomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("invalid room ID"),
		}
	}

	userID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("device.UserID invalid")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("internal server error"),
		}
	}

	var from, to types.StreamPosition
	var limit int
	dir := req.URL.Query().Get("dir")
	if f := req.URL.Query().Get("from"); f != "" {
		if from, err = types.NewStreamPositionFromString(f); err != nil {
			return util.ErrorResponse(err)
		}
	}
	if t := req.URL.Query().Get("to"); t != "" {
		if to, err = types.NewStreamPositionFromString(t); err != nil {
			return util.ErrorResponse(err)
		}
	}
	if l := req.URL.Query().Get("limit"); l != "" {
		if limit, err = strconv.Atoi(l); err != nil {
			return util.ErrorResponse(err)
		}
	}
	if limit == 0 || limit > 50 {
		limit = 50
	}
	if dir == "" {
		dir = "b"
	}
	if dir != "b" && dir != "f" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.MissingParam("Bad or missing dir query parameter (should be either 'b' or 'f')"),
		}
	}

	snapshot, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		logrus.WithError(err).Error("Failed to get snapshot for relations")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)

	res := &RelationsResponse{
		Chunk: []synctypes.ClientEvent{},
	}
	var events []types.StreamEvent
	events, res.PrevBatch, res.NextBatch, err = snapshot.RelationsFor(
		req.Context(), roomID.String(), eventID, relType, eventType, from, to, dir == "b", limit,
	)
	if err != nil {
		return util.ErrorResponse(err)
	}

	headeredEvents := make([]*rstypes.HeaderedEvent, 0, len(events))
	for _, event := range events {
		headeredEvents = append(headeredEvents, event.HeaderedEvent)
	}

	// Apply history visibility to the result events.
	filteredEvents, err := internal.ApplyHistoryVisibilityFilter(req.Context(), snapshot, rsAPI, headeredEvents, nil, *userID, "relations")
	if err != nil {
		return util.ErrorResponse(err)
	}

	// Convert the events into client events, and optionally filter based on the event
	// type if it was specified.
	res.Chunk = make([]synctypes.ClientEvent, 0, len(filteredEvents))
	for _, event := range filteredEvents {
		clientEvent, err := synctypes.ToClientEvent(event.PDU, synctypes.FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(req.Context(), roomID, senderID)
		})
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).WithField("senderID", events[0].SenderID()).WithField("roomID", *roomID).Error("Failed converting to ClientEvent")
			continue
		}
		res.Chunk = append(
			res.Chunk,
			*clientEvent,
		)
	}

	succeeded = true
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}
