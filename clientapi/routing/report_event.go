// Copyright 2023 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/roomserver/api"
	userAPI "github.com/matrix-org/dendrite/userapi/api"
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
