// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package perform

import (
	"context"
	"fmt"

	federationAPI "github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/internal/helpers"
	"github.com/matrix-org/dendrite/roomserver/internal/input"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

type Inviter struct {
	DB      storage.Database
	Cfg     *config.RoomServer
	FSAPI   federationAPI.RoomserverFederationAPI
	Inputer *input.Inputer
}

// nolint:gocyclo
func (r *Inviter) PerformInvite(
	ctx context.Context,
	req *api.PerformInviteRequest,
	res *api.PerformInviteResponse,
) ([]api.OutputEvent, error) {
	var outputUpdates []api.OutputEvent
	event := req.Event
	if event.StateKey() == nil {
		return nil, fmt.Errorf("invite must be a state event")
	}
	_, senderDomain, err := gomatrixserverlib.SplitID('@', event.Sender())
	if err != nil {
		return nil, fmt.Errorf("sender %q is invalid", event.Sender())
	}

	roomID := event.RoomID()
	targetUserID := *event.StateKey()
	info, err := r.DB.RoomInfo(ctx, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to load RoomInfo: %w", err)
	}

	_, domain, err := gomatrixserverlib.SplitID('@', targetUserID)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("The user ID %q is invalid!", targetUserID),
		}
		return nil, nil
	}
	isTargetLocal := r.Cfg.Matrix.IsLocalServerName(domain)
	isOriginLocal := r.Cfg.Matrix.IsLocalServerName(senderDomain)
	if !isOriginLocal && !isTargetLocal {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  "The invite must be either from or to a local user",
		}
		return nil, nil
	}

	logger := util.GetLogger(ctx).WithFields(map[string]interface{}{
		"inviter":  event.Sender(),
		"invitee":  *event.StateKey(),
		"room_id":  roomID,
		"event_id": event.EventID(),
	})
	logger.WithFields(log.Fields{
		"room_version":     req.RoomVersion,
		"room_info_exists": info != nil,
		"target_local":     isTargetLocal,
		"origin_local":     isOriginLocal,
	}).Debug("processing invite event")

	inviteState := req.InviteRoomState
	if len(inviteState) == 0 && info != nil {
		var is []gomatrixserverlib.InviteV2StrippedState
		if is, err = buildInviteStrippedState(ctx, r.DB, info, req); err == nil {
			inviteState = is
		}
	}
	if len(inviteState) == 0 {
		if err = event.SetUnsignedField("invite_room_state", struct{}{}); err != nil {
			return nil, fmt.Errorf("event.SetUnsignedField: %w", err)
		}
	} else {
		if err = event.SetUnsignedField("invite_room_state", inviteState); err != nil {
			return nil, fmt.Errorf("event.SetUnsignedField: %w", err)
		}
	}

	updateMembershipTableManually := func() ([]api.OutputEvent, error) {
		var updater *shared.MembershipUpdater
		if updater, err = r.DB.MembershipUpdater(ctx, roomID, targetUserID, isTargetLocal, req.RoomVersion); err != nil {
			return nil, fmt.Errorf("r.DB.MembershipUpdater: %w", err)
		}
		outputUpdates, err = helpers.UpdateToInviteMembership(updater, &types.Event{
			EventNID: 0,
			Event:    event.Unwrap(),
		}, outputUpdates, req.Event.RoomVersion)
		if err != nil {
			return nil, fmt.Errorf("updateToInviteMembership: %w", err)
		}
		if err = updater.Commit(); err != nil {
			return nil, fmt.Errorf("updater.Commit: %w", err)
		}
		logger.Debugf("updated membership to invite and sending invite OutputEvent")
		return outputUpdates, nil
	}

	if (info == nil || info.IsStub()) && !isOriginLocal && isTargetLocal {
		// The invite came in over federation for a room that we don't know about
		// yet. We need to handle this a bit differently to most invites because
		// we don't know the room state, therefore the roomserver can't process
		// an input event. Instead we will update the membership table with the
		// new invite and generate an output event.
		return updateMembershipTableManually()
	}

	var isAlreadyJoined bool
	if info != nil {
		_, isAlreadyJoined, _, err = r.DB.GetMembership(ctx, info.RoomNID, *event.StateKey())
		if err != nil {
			return nil, fmt.Errorf("r.DB.GetMembership: %w", err)
		}
	}
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
		res.Error = &api.PerformError{
			Code: api.PerformErrorNotAllowed,
			Msg:  "User is already joined to room",
		}
		logger.Debugf("user already joined")
		return nil, nil
	}

	// If the invite originated remotely then we can't send an
	// InputRoomEvent for the invite as it will never pass auth checks
	// due to lacking room state, but we still need to tell the client
	// about the invite so we can accept it, hence we return an output
	// event to send to the Sync API.
	if !isOriginLocal {
		return updateMembershipTableManually()
	}

	// The invite originated locally. Therefore we have a responsibility to
	// try and see if the user is allowed to make this invite. We can't do
	// this for invites coming in over federation - we have to take those on
	// trust.
	_, err = helpers.CheckAuthEvents(ctx, r.DB, event, event.AuthEventIDs())
	if err != nil {
		logger.WithError(err).WithField("event_id", event.EventID()).WithField("auth_event_ids", event.AuthEventIDs()).Error(
			"processInviteEvent.checkAuthEvents failed for event",
		)
		res.Error = &api.PerformError{
			Msg:  err.Error(),
			Code: api.PerformErrorNotAllowed,
		}
		return nil, nil
	}

	// If the invite originated from us and the target isn't local then we
	// should try and send the invite over federation first. It might be
	// that the remote user doesn't exist, in which case we can give up
	// processing here.
	if req.SendAsServer != api.DoNotSendToOtherServers && !isTargetLocal {
		fsReq := &federationAPI.PerformInviteRequest{
			RoomVersion:     req.RoomVersion,
			Event:           event,
			InviteRoomState: inviteState,
		}
		fsRes := &federationAPI.PerformInviteResponse{}
		if err = r.FSAPI.PerformInvite(ctx, fsReq, fsRes); err != nil {
			res.Error = &api.PerformError{
				Msg:  err.Error(),
				Code: api.PerformErrorNotAllowed,
			}
			logger.WithError(err).WithField("event_id", event.EventID()).Error("r.FSAPI.PerformInvite failed")
			return nil, nil
		}
		event = fsRes.Event
		logger.Debugf("Federated PerformInvite success with event ID %s", event.EventID())
	}

	// Send the invite event to the roomserver input stream. This will
	// notify existing users in the room about the invite, update the
	// membership table and ensure that the event is ready and available
	// to use as an auth event when accepting the invite.
	// It will NOT notify the invitee of this invite.
	inputReq := &api.InputRoomEventsRequest{
		InputRoomEvents: []api.InputRoomEvent{
			{
				Kind:         api.KindNew,
				Event:        event,
				Origin:       senderDomain,
				SendAsServer: req.SendAsServer,
			},
		},
	}
	inputRes := &api.InputRoomEventsResponse{}
	if err = r.Inputer.InputRoomEvents(context.Background(), inputReq, inputRes); err != nil {
		return nil, fmt.Errorf("r.Inputer.InputRoomEvents: %w", err)
	}
	if err = inputRes.Err(); err != nil {
		res.Error = &api.PerformError{
			Msg:  fmt.Sprintf("r.InputRoomEvents: %s", err.Error()),
			Code: api.PerformErrorNotAllowed,
		}
		logger.WithError(err).WithField("event_id", event.EventID()).Error("r.InputRoomEvents failed")
	}

	// Don't notify the sync api of this event in the same way as a federated invite so the invitee
	// gets the invite, as the roomserver will do this when it processes the m.room.member invite.
	return outputUpdates, nil
}

func buildInviteStrippedState(
	ctx context.Context,
	db storage.Database,
	info *types.RoomInfo,
	input *api.PerformInviteRequest,
) ([]gomatrixserverlib.InviteV2StrippedState, error) {
	stateWanted := []gomatrixserverlib.StateKeyTuple{}
	// "If they are set on the room, at least the state for m.room.avatar, m.room.canonical_alias, m.room.join_rules, and m.room.name SHOULD be included."
	// https://matrix.org/docs/spec/client_server/r0.6.0#m-room-member
	for _, t := range []string{
		gomatrixserverlib.MRoomName, gomatrixserverlib.MRoomCanonicalAlias,
		gomatrixserverlib.MRoomJoinRules, gomatrixserverlib.MRoomAvatar,
		gomatrixserverlib.MRoomEncryption, gomatrixserverlib.MRoomCreate,
	} {
		stateWanted = append(stateWanted, gomatrixserverlib.StateKeyTuple{
			EventType: t,
			StateKey:  "",
		})
	}
	roomState := state.NewStateResolution(db, info)
	stateEntries, err := roomState.LoadStateAtSnapshotForStringTuples(
		ctx, info.StateSnapshotNID(), stateWanted,
	)
	if err != nil {
		return nil, err
	}
	stateNIDs := []types.EventNID{}
	for _, stateNID := range stateEntries {
		stateNIDs = append(stateNIDs, stateNID.EventNID)
	}
	stateEvents, err := db.Events(ctx, stateNIDs)
	if err != nil {
		return nil, err
	}
	inviteState := []gomatrixserverlib.InviteV2StrippedState{
		gomatrixserverlib.NewInviteV2StrippedState(input.Event.Event),
	}
	stateEvents = append(stateEvents, types.Event{Event: input.Event.Unwrap()})
	for _, event := range stateEvents {
		inviteState = append(inviteState, gomatrixserverlib.NewInviteV2StrippedState(event.Event))
	}
	return inviteState, nil
}
