// Copyright 2020-2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// Peek implements the SS /peek API, handling inbound peeks
func Peek(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	roomID, peekID string,
	remoteVersions []gomatrixserverlib.RoomVersion,
) util.JSONResponse {
	// TODO: check if we're just refreshing an existing peek by querying the federationapi
	roomVersion, err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), roomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Check that the room that the peeking server is trying to peek is actually
	// one of the room versions that they listed in their supported ?ver= in
	// the peek URL.
	remoteSupportsVersion := false
	for _, v := range remoteVersions {
		if v == roomVersion {
			remoteSupportsVersion = true
			break
		}
	}
	// If it isn't, stop trying to peek the room.
	if !remoteSupportsVersion {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.IncompatibleRoomVersion(string(roomVersion)),
		}
	}

	// TODO: Check history visibility

	// tell the peeking server to renew every hour
	renewalInterval := int64(60 * 60 * 1000 * 1000)

	var response api.PerformInboundPeekResponse
	err = rsAPI.PerformInboundPeek(
		httpReq.Context(),
		&api.PerformInboundPeekRequest{
			RoomID:          roomID,
			PeekID:          peekID,
			ServerName:      request.Origin(),
			RenewalInterval: renewalInterval,
		},
		&response,
	)
	if err != nil {
		resErr := util.ErrorResponse(err)
		return resErr
	}

	if !response.RoomExists {
		return util.JSONResponse{Code: http.StatusNotFound, JSON: nil}
	}

	respPeek := fclient.RespPeek{
		StateEvents:     types.NewEventJSONsFromHeaderedEvents(response.StateEvents),
		AuthEvents:      types.NewEventJSONsFromHeaderedEvents(response.AuthChainEvents),
		RoomVersion:     response.RoomVersion,
		LatestEvent:     response.LatestEvent.PDU,
		RenewalInterval: renewalInterval,
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: respPeek,
	}
}
