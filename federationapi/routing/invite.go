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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

// InviteV2 implements /_matrix/federation/v2/invite/{roomID}/{eventID}
func InviteV2(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	roomID spec.RoomID,
	eventID string,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
) util.JSONResponse {
	inviteReq := fclient.InviteV2Request{}
	err := json.Unmarshal(request.Content(), &inviteReq)
	switch e := err.(type) {
	case gomatrixserverlib.UnsupportedRoomVersionError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.UnsupportedRoomVersion(
				fmt.Sprintf("Room version %q is not supported by this server.", e.Version),
			),
		}
	case gomatrixserverlib.BadJSONError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(err.Error()),
		}
	case nil:
		if inviteReq.Event().StateKey() == nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON("The invite event has no state key"),
			}
		}

		invitedUser, userErr := spec.NewUserID(*inviteReq.Event().StateKey(), true)
		if userErr != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("The user ID is invalid"),
			}
		}
		if !cfg.Matrix.IsLocalServerName(invitedUser.Domain()) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam("The invited user domain does not belong to this server"),
			}
		}

		input := gomatrixserverlib.HandleInviteInput{
			Context:           httpReq.Context(),
			RoomVersion:       inviteReq.RoomVersion(),
			RoomID:            roomID,
			EventID:           eventID,
			InvitedUser:       *invitedUser,
			KeyID:             cfg.Matrix.KeyID,
			PrivateKey:        cfg.Matrix.PrivateKey,
			Verifier:          keys,
			InviteQuerier:     rsAPI,
			MembershipQuerier: &api.MembershipQuerier{Roomserver: rsAPI},
			StateQuerier:      rsAPI.StateQuerier(),
			InviteEvent:       inviteReq.Event(),
			StrippedState:     inviteReq.InviteRoomState(),
		}
		event, jsonErr := handleInvite(input, rsAPI)
		if jsonErr != nil {
			return *jsonErr
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: fclient.RespInviteV2{Event: event.JSON()},
		}
	default:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.NotJSON("The request body could not be decoded into an invite request. " + err.Error()),
		}
	}
}

// InviteV1 implements /_matrix/federation/v1/invite/{roomID}/{eventID}
func InviteV1(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	roomID spec.RoomID,
	eventID string,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
) util.JSONResponse {
	roomVer := gomatrixserverlib.RoomVersionV1
	body := request.Content()
	// roomVer is hardcoded to v1 so we know we won't panic on Must
	event, err := gomatrixserverlib.MustGetRoomVersion(roomVer).NewEventFromTrustedJSON(body, false)
	switch err.(type) {
	case gomatrixserverlib.BadJSONError:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(err.Error()),
		}
	case nil:
	default:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.NotJSON("The request body could not be decoded into an invite v1 request. " + err.Error()),
		}
	}
	var strippedState []gomatrixserverlib.InviteStrippedState
	if jsonErr := json.Unmarshal(event.Unsigned(), &strippedState); jsonErr != nil {
		// just warn, they may not have added any.
		util.GetLogger(httpReq.Context()).Warnf("failed to extract stripped state from invite event")
	}

	if event.StateKey() == nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The invite event has no state key"),
		}
	}

	invitedUser, err := spec.NewUserID(*event.StateKey(), true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("The user ID is invalid"),
		}
	}
	if !cfg.Matrix.IsLocalServerName(invitedUser.Domain()) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("The invited user domain does not belong to this server"),
		}
	}

	input := gomatrixserverlib.HandleInviteInput{
		Context:           httpReq.Context(),
		RoomVersion:       roomVer,
		RoomID:            roomID,
		EventID:           eventID,
		InvitedUser:       *invitedUser,
		KeyID:             cfg.Matrix.KeyID,
		PrivateKey:        cfg.Matrix.PrivateKey,
		Verifier:          keys,
		InviteQuerier:     rsAPI,
		MembershipQuerier: &api.MembershipQuerier{Roomserver: rsAPI},
		StateQuerier:      rsAPI.StateQuerier(),
		InviteEvent:       event,
		StrippedState:     strippedState,
	}
	event, jsonErr := handleInvite(input, rsAPI)
	if jsonErr != nil {
		return *jsonErr
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: fclient.RespInvite{Event: event.JSON()},
	}
}

func handleInvite(input gomatrixserverlib.HandleInviteInput, rsAPI api.FederationRoomserverAPI) (gomatrixserverlib.PDU, *util.JSONResponse) {
	inviteEvent, err := gomatrixserverlib.HandleInvite(input)
	switch e := err.(type) {
	case nil:
	case spec.InternalServerError:
		util.GetLogger(input.Context).WithError(err)
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	case spec.MatrixError:
		util.GetLogger(input.Context).WithError(err)
		code := http.StatusInternalServerError
		switch e.ErrCode {
		case spec.ErrorForbidden:
			code = http.StatusForbidden
		case spec.ErrorUnsupportedRoomVersion:
			fallthrough // http.StatusBadRequest
		case spec.ErrorBadJSON:
			code = http.StatusBadRequest
		}

		return nil, &util.JSONResponse{
			Code: code,
			JSON: e,
		}
	default:
		util.GetLogger(input.Context).WithError(err)
		return nil, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("unknown error"),
		}
	}

	headeredInvite := &types.HeaderedEvent{PDU: inviteEvent}
	if err = rsAPI.HandleInvite(input.Context, headeredInvite); err != nil {
		util.GetLogger(input.Context).WithError(err).Error("HandleInvite failed")
		return nil, &util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	return inviteEvent, nil
}
