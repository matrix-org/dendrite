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
	roomID string,
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
		return processInvite(
			httpReq.Context(), true, inviteReq.Event(), inviteReq.RoomVersion(), inviteReq.InviteRoomState(), roomID, eventID, cfg, rsAPI, keys,
		)
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
	roomID string,
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
	return processInvite(
		httpReq.Context(), false, event, roomVer, strippedState, roomID, eventID, cfg, rsAPI, keys,
	)
}

func processInvite(
	ctx context.Context,
	isInviteV2 bool,
	event gomatrixserverlib.PDU,
	roomVer gomatrixserverlib.RoomVersion,
	strippedState []fclient.InviteV2StrippedState,
	roomID string,
	eventID string,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
) util.JSONResponse {

	// Check that we can accept invites for this room version.
	verImpl, err := gomatrixserverlib.GetRoomVersion(roomVer)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.UnsupportedRoomVersion(
				fmt.Sprintf("Room version %q is not supported by this server.", roomVer),
			),
		}
	}

	// Check that the room ID is correct.
	if event.RoomID() != roomID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The room ID in the request path must match the room ID in the invite event JSON"),
		}
	}

	// Check that the event ID is correct.
	if event.EventID() != eventID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The event ID in the request path must match the event ID in the invite event JSON"),
		}
	}

	if event.StateKey() == nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The invite event has no state key"),
		}
	}

	_, domain, err := cfg.Matrix.SplitLocalID('@', *event.StateKey())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam(fmt.Sprintf("The user ID is invalid or domain %q does not belong to this server", domain)),
		}
	}

	// Check that the event is signed by the server sending the request.
	redacted, err := verImpl.RedactEventJSON(event.JSON())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The event JSON could not be redacted"),
		}
	}
	_, serverName, err := gomatrixserverlib.SplitID('@', event.Sender())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The event JSON contains an invalid sender"),
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if verifyResults[0].Error != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("The invite must be signed by the server it originated on"),
		}
	}

	// Sign the event so that other servers will know that we have received the invite.
	signedEvent := event.Sign(
		string(domain), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey,
	)

	inviteEvent := &types.HeaderedEvent{PDU: signedEvent}
	err = handleInvite(ctx, cfg, inviteEvent, strippedState, rsAPI)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("HandleInvite failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Add the invite event to the roomserver.
	if err = rsAPI.HandleInvite(ctx, inviteEvent); err != nil {
		util.GetLogger(ctx).WithError(err).Error("HandleInvite failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	// Return the signed event to the originating server, it should then tell
	// the other servers in the room that we have been invited.
	if isInviteV2 {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: fclient.RespInviteV2{Event: signedEvent.JSON()},
		}
	} else {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: fclient.RespInvite{Event: signedEvent.JSON()},
		}
	}
}

// TODO: Define interface instead of passing in FedRoomserverAPI
// TODO: Migrate to GMSL
// TODO: Clean up logic/naming of stuff

func handleInvite(
	ctx context.Context,
	cfg *config.FederationAPI,
	inviteEvent *types.HeaderedEvent,
	inviteRoomState []fclient.InviteV2StrippedState,
	rsAPI api.FederationRoomserverAPI,
) error {
	if inviteEvent.StateKey() == nil {
		return fmt.Errorf("invite must be a state event")
	}

	roomID := inviteEvent.RoomID()
	targetUserID := *inviteEvent.StateKey()
	_, domain, err := gomatrixserverlib.SplitID('@', targetUserID)
	if err != nil {
		return api.ErrInvalidID{Err: fmt.Errorf("the user ID %s is invalid", targetUserID)}
	}
	isTargetLocal := cfg.Matrix.IsLocalServerName(domain)
	if !isTargetLocal {
		return api.ErrInvalidID{Err: fmt.Errorf("the invite must be to a local user")}
	}

	validRoomID, err := spec.NewRoomID(roomID)
	if err != nil {
		return err
	}
	isKnownRoom, err := rsAPI.IsKnownRoom(ctx, *validRoomID)
	if err != nil {
		return err
	}

	inviteState := inviteRoomState
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
		if is, err := rsAPI.GenerateInviteStrippedStateV2(ctx, *validRoomID, stateWanted, inviteEvent); err == nil {
			inviteState = is
		} else {
			return err
		}
	}

	logger := util.GetLogger(ctx).WithFields(map[string]interface{}{
		"inviter":  inviteEvent.Sender(),
		"invitee":  *inviteEvent.StateKey(),
		"room_id":  roomID,
		"event_id": inviteEvent.EventID(),
	})
	logger.WithFields(logrus.Fields{
		"room_version":     inviteEvent.Version(),
		"room_info_exists": isKnownRoom,
		"target_local":     isTargetLocal,
	}).Debug("processing incoming federation invite event")

	if len(inviteState) == 0 {
		if err = inviteEvent.SetUnsignedField("invite_room_state", struct{}{}); err != nil {
			return fmt.Errorf("event.SetUnsignedField: %w", err)
		}
	} else {
		if err = inviteEvent.SetUnsignedField("invite_room_state", inviteState); err != nil {
			return fmt.Errorf("event.SetUnsignedField: %w", err)
		}
	}

	if !isKnownRoom && isTargetLocal {
		// The invite came in over federation for a room that we don't know about
		// yet. We need to handle this a bit differently to most invites because
		// we don't know the room state, therefore the roomserver can't process
		// an input event. Instead we will update the membership table with the
		// new invite and generate an output event.
		return nil
	}

	req := api.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: targetUserID,
	}
	res := api.QueryMembershipForUserResponse{}
	err = rsAPI.QueryMembershipForUser(ctx, &req, &res)
	if err != nil {
		return fmt.Errorf("r.QueryMembershipForUser: %w", err)
	}
	isAlreadyJoined := (res.Membership == spec.Join)

	//_, isAlreadyJoined, _, err = r.DB.GetMembership(ctx, info.RoomNID, *inviteEvent.StateKey())
	//if err != nil {
	//	return nil, fmt.Errorf("r.DB.GetMembership: %w", err)
	//}
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
		return api.ErrNotAllowed{Err: fmt.Errorf("user is already joined to room")}
	}

	return nil
}
