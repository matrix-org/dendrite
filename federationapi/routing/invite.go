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
	"crypto/ed25519"
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
	"github.com/sirupsen/logrus"
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

		invitedUser, err := spec.NewUserID(*inviteReq.Event().StateKey(), true)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam(fmt.Sprintf("The user ID is invalid")),
			}
		}
		if !cfg.Matrix.IsLocalServerName(invitedUser.Domain()) {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.InvalidParam(fmt.Sprintf("The invited user domain does not belong to this server")),
			}
		}

		input := HandleInviteInput{
			Context:               httpReq.Context(),
			RoomVersion:           inviteReq.RoomVersion(),
			RoomID:                roomID,
			EventID:               eventID,
			InvitedUser:           *invitedUser,
			KeyID:                 cfg.Matrix.KeyID,
			PrivateKey:            cfg.Matrix.PrivateKey,
			Verifier:              keys,
			InviteQuerier:         rsAPI,
			MembershipQuerier:     MembershipQuerier{roomserver: rsAPI},
			GenerateStrippedState: rsAPI.GenerateInviteStrippedState,
			InviteEvent:           inviteReq.Event(),
			StrippedState:         inviteReq.InviteRoomState(),
		}
		event, jsonErr := handleInvite(input, rsAPI)
		if err != nil {
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
	var strippedState []fclient.InviteV2StrippedState
	if err := json.Unmarshal(event.Unsigned(), &strippedState); err != nil {
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
			JSON: spec.InvalidParam(fmt.Sprintf("The user ID is invalid")),
		}
	}
	if !cfg.Matrix.IsLocalServerName(invitedUser.Domain()) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(fmt.Sprintf("The invited user domain does not belong to this server")),
		}
	}

	input := HandleInviteInput{
		Context:               httpReq.Context(),
		RoomVersion:           roomVer,
		RoomID:                roomID,
		EventID:               eventID,
		InvitedUser:           *invitedUser,
		KeyID:                 cfg.Matrix.KeyID,
		PrivateKey:            cfg.Matrix.PrivateKey,
		Verifier:              keys,
		InviteQuerier:         rsAPI,
		MembershipQuerier:     MembershipQuerier{roomserver: rsAPI},
		GenerateStrippedState: rsAPI.GenerateInviteStrippedState,
		InviteEvent:           event,
		StrippedState:         strippedState,
	}
	event, jsonErr := handleInvite(input, rsAPI)
	if err != nil {
		return *jsonErr
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: fclient.RespInvite{Event: event.JSON()},
	}
}

func handleInvite(input HandleInviteInput, rsAPI api.FederationRoomserverAPI) (gomatrixserverlib.PDU, *util.JSONResponse) {
	inviteEvent, err := processInvite(input)
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

// TODO: Migrate to GMSL

type RoomQuerier interface {
	IsKnownRoom(ctx context.Context, roomID spec.RoomID) (bool, error)
}

type HandleInviteInput struct {
	Context           context.Context
	RoomVersion       gomatrixserverlib.RoomVersion
	RoomID            spec.RoomID
	EventID           string
	InvitedUser       spec.UserID
	KeyID             gomatrixserverlib.KeyID
	PrivateKey        ed25519.PrivateKey
	Verifier          gomatrixserverlib.JSONVerifier
	InviteQuerier     RoomQuerier
	MembershipQuerier MembershipQuerier
	// TODO: get rid of fclient references if possible
	GenerateStrippedState func(ctx context.Context, roomID spec.RoomID, stateWanted []gomatrixserverlib.StateKeyTuple, inviteEvent gomatrixserverlib.PDU) ([]fclient.InviteV2StrippedState, error)

	InviteEvent   gomatrixserverlib.PDU
	StrippedState []fclient.InviteV2StrippedState
}

func processInvite(
	input HandleInviteInput,
) (gomatrixserverlib.PDU, error) {
	// Check that we can accept invites for this room version.
	verImpl, err := gomatrixserverlib.GetRoomVersion(input.RoomVersion)
	if err != nil {
		return nil, spec.UnsupportedRoomVersion(
			fmt.Sprintf("Room version %q is not supported by this server.", input.RoomVersion),
		)
	}

	// Check that the room ID is correct.
	if input.InviteEvent.RoomID() != input.RoomID.String() {
		return nil, spec.BadJSON("The room ID in the request path must match the room ID in the invite event JSON")
	}

	// Check that the event ID is correct.
	if input.InviteEvent.EventID() != input.EventID {
		return nil, spec.BadJSON("The event ID in the request path must match the event ID in the invite event JSON")
	}

	// Check that the event is signed by the server sending the request.
	redacted, err := verImpl.RedactEventJSON(input.InviteEvent.JSON())
	if err != nil {
		return nil, spec.BadJSON("The event JSON could not be redacted")
	}

	sender, err := spec.NewUserID(input.InviteEvent.Sender(), true)
	if err != nil {
		return nil, spec.BadJSON("The event JSON contains an invalid sender")
	}
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName:             sender.Domain(),
		Message:                redacted,
		AtTS:                   input.InviteEvent.OriginServerTS(),
		StrictValidityChecking: true,
	}}
	verifyResults, err := input.Verifier.VerifyJSONs(input.Context, verifyRequests)
	if err != nil {
		util.GetLogger(input.Context).WithError(err).Error("keys.VerifyJSONs failed")
		return nil, spec.InternalServerError{}
	}
	if verifyResults[0].Error != nil {
		return nil, spec.Forbidden("The invite must be signed by the server it originated on")
	}

	// Sign the event so that other servers will know that we have received the invite.
	signedEvent := input.InviteEvent.Sign(
		string(input.InvitedUser.Domain()), input.KeyID, input.PrivateKey,
	)

	inviteEvent := &types.HeaderedEvent{PDU: signedEvent}

	if inviteEvent.StateKey() == nil {
		util.GetLogger(input.Context).Error("invite must be a state event")
		return nil, spec.InternalServerError{}
	}

	isKnownRoom, err := input.InviteQuerier.IsKnownRoom(input.Context, input.RoomID)
	if err != nil {
		util.GetLogger(input.Context).WithError(err).Error("failed querying known room")
		return nil, spec.InternalServerError{}
	}

	inviteState := input.StrippedState
	if len(inviteState) == 0 {
		// "If they are set on the room, at least the state for m.room.avatar, m.room.canonical_alias, m.room.join_rules, and m.room.name SHOULD be included."
		// https://matrix.org/docs/spec/client_server/r0.6.0#m-room-member
		stateWanted := []gomatrixserverlib.StateKeyTuple{}
		for _, t := range []string{
			spec.MRoomName, spec.MRoomCanonicalAlias,
			spec.MRoomJoinRules, spec.MRoomAvatar,
			spec.MRoomEncryption, spec.MRoomCreate,
		} {
			stateWanted = append(stateWanted, gomatrixserverlib.StateKeyTuple{
				EventType: t,
				StateKey:  "",
			})
		}
		if is, err := input.GenerateStrippedState(input.Context, input.RoomID, stateWanted, inviteEvent); err == nil {
			inviteState = is
		} else {
			util.GetLogger(input.Context).WithError(err).Error("failed querying known room")
			return nil, spec.InternalServerError{}
		}
	}

	logger := util.GetLogger(input.Context).WithFields(map[string]interface{}{
		"inviter":  inviteEvent.Sender(),
		"invitee":  *inviteEvent.StateKey(),
		"room_id":  input.RoomID.String(),
		"event_id": inviteEvent.EventID(),
	})
	logger.WithFields(logrus.Fields{
		"room_version":     inviteEvent.Version(),
		"room_info_exists": isKnownRoom,
	}).Debug("processing incoming federation invite event")

	if len(inviteState) == 0 {
		if err = inviteEvent.SetUnsignedField("invite_room_state", struct{}{}); err != nil {
			util.GetLogger(input.Context).WithError(err).Error("failed setting unsigned field")
			return nil, spec.InternalServerError{}
		}
	} else {
		if err = inviteEvent.SetUnsignedField("invite_room_state", inviteState); err != nil {
			util.GetLogger(input.Context).WithError(err).Error("failed setting unsigned field")
			return nil, spec.InternalServerError{}
		}
	}

	if isKnownRoom {
		membership, err := input.MembershipQuerier.CurrentMembership(input.Context, input.RoomID, input.InvitedUser)
		if err != nil {
			util.GetLogger(input.Context).WithError(err).Error("failed getting user membership")
			return nil, spec.InternalServerError{}

		}
		isAlreadyJoined := (membership == spec.Join)

		if isAlreadyJoined {
			// If the user is joined to the room then that takes precedence over this
			// invite event. It makes little sense to move a user that is already
			// joined to the room into the invite state.
			// This could plausibly happen if an invite request raced with a join
			// request for a user. For example if a user was invited to a public
			// room and they joined the room at the same time as the invite was sent.
			// The other way this could plausibly happen is if an invite raced with
			// a kick. For example if a user was kicked from a room in error and in
			// response someone else in the room re-invited them then it is possible
			// for the invite request to race with the leave event so that the
			// target receives invite before it learns that it has been kicked.
			// There are a few ways this could be plausibly handled in the roomserver.
			// 1) Store the invite, but mark it as retired. That will result in the
			//    permanent rejection of that invite event. So even if the target
			//    user leaves the room and the invite is retransmitted it will be
			//    ignored. However a new invite with a new event ID would still be
			//    accepted.
			// 2) Silently discard the invite event. This means that if the event
			//    was retransmitted at a later date after the target user had left
			//    the room we would accept the invite. However since we hadn't told
			//    the sending server that the invite had been discarded it would
			//    have no reason to attempt to retry.
			// 3) Signal the sending server that the user is already joined to the
			//    room.
			// For now we will implement option 2. Since in the abesence of a retry
			// mechanism it will be equivalent to option 1, and we don't have a
			// signalling mechanism to implement option 3.
			logger.Debugf("user already joined")
			util.GetLogger(input.Context).Error("user is already joined to room")
			return nil, spec.InternalServerError{}
		}
	}

	return inviteEvent.PDU, nil
}
