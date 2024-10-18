// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/roomserver/api"
	userAPI "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type reportEventRequest struct {
	Reason string `json:"reason"`
	Score  int64  `json:"score"`
}

func ReportEvent(
	req *http.Request,
	device *userAPI.Device,
	roomID, eventID string,
	rsAPI api.ClientRoomserverAPI,
) util.JSONResponse {
	defer req.Body.Close() // nolint: errcheck

	deviceUserID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.NotFound("You don't have permission to report this event, bad userID"),
		}
	}
	// The requesting user must be a member of the room
	errRes := checkMemberInRoom(req.Context(), rsAPI, *deviceUserID, roomID)
	if errRes != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound, // Spec demands this...
			JSON: spec.NotFound("The event was not found or you are not joined to the room."),
		}
	}

	// Parse the request
	report := reportEventRequest{}
	if resErr := httputil.UnmarshalJSONRequest(req, &report); resErr != nil {
		return *resErr
	}

	queryRes := &api.QueryEventsByIDResponse{}
	if err = rsAPI.QueryEventsByID(req.Context(), &api.QueryEventsByIDRequest{
		RoomID:   roomID,
		EventIDs: []string{eventID},
	}, queryRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{Err: err.Error()},
		}
	}

	// No event was found or it was already redacted
	if len(queryRes.Events) == 0 || queryRes.Events[0].Redacted() {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("The event was not found or you are not joined to the room."),
		}
	}

	_, err = rsAPI.InsertReportedEvent(req.Context(), roomID, eventID, device.UserID, report.Reason, report.Score)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{Err: err.Error()},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
