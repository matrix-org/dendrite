// Copyright 2017 Vector Creations Ltd
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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	roomserverVersion "github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// InviteV2 implements /_matrix/federation/v2/invite/{roomID}/{eventID}
func InviteV2(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	roomID string,
	eventID string,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
) util.JSONResponse {
	inviteReq := gomatrixserverlib.InviteV2Request{}
	err := json.Unmarshal(request.Content(), &inviteReq)
	switch e := err.(type) {
	case gomatrixserverlib.UnsupportedRoomVersionError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion(
				fmt.Sprintf("Room version %q is not supported by this server.", e.Version),
			),
		}
	case gomatrixserverlib.BadJSONError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	case nil:
		return processInvite(
			httpReq.Context(), true, inviteReq.Event(), inviteReq.RoomVersion(), inviteReq.InviteRoomState(), roomID, eventID, cfg, rsAPI, keys,
		)
	default:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into an invite request. " + err.Error()),
		}
	}
}

// InviteV1 implements /_matrix/federation/v1/invite/{roomID}/{eventID}
func InviteV1(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	roomID string,
	eventID string,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
) util.JSONResponse {
	roomVer := gomatrixserverlib.RoomVersionV1
	body := request.Content()
	event, err := gomatrixserverlib.NewEventFromTrustedJSON(body, false, roomVer)
	switch err.(type) {
	case gomatrixserverlib.BadJSONError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(err.Error()),
		}
	case nil:
	default:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into an invite v1 request. " + err.Error()),
		}
	}
	var strippedState []gomatrixserverlib.InviteV2StrippedState
	if err := json.Unmarshal(event.Unsigned(), &strippedState); err != nil {
		// just warn, they may not have added any.
		util.GetLogger(httpReq.Context()).Warnf("failed to extract stripped state from invite event")
	}
	return processInvite(
		httpReq.Context(), false, event, roomVer, strippedState, roomID, eventID, cfg, rsAPI, keys,
	)
}

func processInvite(
	ctx context.Context,
	isInviteV2 bool,
	event *gomatrixserverlib.Event,
	roomVer gomatrixserverlib.RoomVersion,
	strippedState []gomatrixserverlib.InviteV2StrippedState,
	roomID string,
	eventID string,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
) util.JSONResponse {

	// Check that we can accept invites for this room version.
	if _, err := roomserverVersion.SupportedRoomVersion(roomVer); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnsupportedRoomVersion(
				fmt.Sprintf("Room version %q is not supported by this server.", roomVer),
			),
		}
	}

	// Check that the room ID is correct.
	if event.RoomID() != roomID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The room ID in the request path must match the room ID in the invite event JSON"),
		}
	}

	// Check that the event ID is correct.
	if event.EventID() != eventID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event ID in the request path must match the event ID in the invite event JSON"),
		}
	}

	if event.StateKey() == nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The invite event has no state key"),
		}
	}

	_, domain, err := cfg.Matrix.SplitLocalID('@', *event.StateKey())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(fmt.Sprintf("The user ID is invalid or domain %q does not belong to this server", domain)),
		}
	}

	// Check that the event is signed by the server sending the request.
	redacted, err := gomatrixserverlib.RedactEventJSON(event.JSON(), event.Version())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event JSON could not be redacted"),
		}
	}
	_, serverName, err := gomatrixserverlib.SplitID('@', event.Sender())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The event JSON contains an invalid sender"),
		}
	}
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName:             serverName,
		Message:                redacted,
		AtTS:                   event.OriginServerTS(),
		StrictValidityChecking: true,
	}}
	verifyResults, err := keys.VerifyJSONs(ctx, verifyRequests)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("keys.VerifyJSONs failed")
		return jsonerror.InternalServerError()
	}
	if verifyResults[0].Error != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The invite must be signed by the server it originated on"),
		}
	}

	// Sign the event so that other servers will know that we have received the invite.
	signedEvent := event.Sign(
		string(domain), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)

	// Add the invite event to the roomserver.
	inviteEvent := signedEvent.Headered(roomVer)
	request := &api.PerformInviteRequest{
		Event:           inviteEvent,
		InviteRoomState: strippedState,
		RoomVersion:     inviteEvent.RoomVersion,
		SendAsServer:    string(api.DoNotSendToOtherServers),
		TransactionID:   nil,
	}
	response := &api.PerformInviteResponse{}
	if err := rsAPI.PerformInvite(ctx, request, response); err != nil {
		util.GetLogger(ctx).WithError(err).Error("PerformInvite failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.InternalServerError(),
		}
	}
	if response.Error != nil {
		return response.Error.JSONResponse()
	}
	// Return the signed event to the originating server, it should then tell
	// the other servers in the room that we have been invited.
	if isInviteV2 {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: gomatrixserverlib.RespInviteV2{Event: signedEvent.JSON()},
		}
	} else {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: gomatrixserverlib.RespInvite{Event: signedEvent.JSON()},
		}
	}
}
