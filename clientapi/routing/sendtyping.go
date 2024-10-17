// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only
// Please see LICENSE in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/producers"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type typingContentJSON struct {
	Typing  bool  `json:"typing"`
	Timeout int64 `json:"timeout"`
}

// SendTyping handles PUT /rooms/{roomID}/typing/{userID}
// sends the typing events to client API typingProducer
func SendTyping(
	req *http.Request, device *userapi.Device, roomID string,
	userID string, rsAPI roomserverAPI.ClientRoomserverAPI,
	syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {
	if device.UserID != userID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Cannot set another user's typing state"),
		}
	}

	deviceUserID, err := spec.NewUserID(userID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID doesn't have power level to change visibility"),
		}
	}

	// Verify that the user is a member of this room
	resErr := checkMemberInRoom(req.Context(), rsAPI, *deviceUserID, roomID)
	if resErr != nil {
		return *resErr
	}

	// parse the incoming http request
	var r typingContentJSON
	resErr = httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	if err := syncProducer.SendTyping(req.Context(), userID, roomID, r.Typing, r.Timeout); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("eduProducer.Send failed")
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
