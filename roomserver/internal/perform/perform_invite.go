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
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

type Inviter struct {
	DB      storage.Database
	Cfg     *config.RoomServer
	FSAPI   federationAPI.RoomserverFederationAPI
	RSAPI   api.RoomserverInternalAPI
	Inputer *input.Inputer
}

func (r *Inviter) IsKnownRoom(ctx context.Context, roomID spec.RoomID) (bool, error) {
	info, err := r.DB.RoomInfo(ctx, roomID.String())
	if err != nil {
		return false, fmt.Errorf("failed to load RoomInfo: %w", err)
	}
	return (info != nil && !info.IsStub()), nil
}

func (r *Inviter) GenerateInviteStrippedState(
	ctx context.Context, roomID spec.RoomID, stateWanted []gomatrixserverlib.StateKeyTuple, inviteEvent gomatrixserverlib.PDU,
) ([]gomatrixserverlib.InviteStrippedState, error) {
	info, err := r.DB.RoomInfo(ctx, roomID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to load RoomInfo: %w", err)
	}
	if info != nil {
		roomState := state.NewStateResolution(r.DB, info)
		stateEntries, err := roomState.LoadStateAtSnapshotForStringTuples(
			ctx, info.StateSnapshotNID(), stateWanted,
		)
		if err != nil {
			return nil, nil
		}
		stateNIDs := []types.EventNID{}
		for _, stateNID := range stateEntries {
			stateNIDs = append(stateNIDs, stateNID.EventNID)
		}
		stateEvents, err := r.DB.Events(ctx, info.RoomVersion, stateNIDs)
		if err != nil {
			return nil, nil
		}
		inviteState := []gomatrixserverlib.InviteStrippedState{
			gomatrixserverlib.NewInviteStrippedState(inviteEvent),
		}
		stateEvents = append(stateEvents, types.Event{PDU: inviteEvent})
		for _, event := range stateEvents {
			inviteState = append(inviteState, gomatrixserverlib.NewInviteStrippedState(event.PDU))
		}
		return inviteState, nil
	}
	return nil, nil
}

func (r *Inviter) ProcessInviteMembership(
	ctx context.Context, inviteEvent *types.HeaderedEvent,
) ([]api.OutputEvent, error) {
	var outputUpdates []api.OutputEvent
	var updater *shared.MembershipUpdater
	_, domain, err := gomatrixserverlib.SplitID('@', *inviteEvent.StateKey())
	if err != nil {
		return nil, api.ErrInvalidID{Err: fmt.Errorf("the user ID %s is invalid", *inviteEvent.StateKey())}
	}
	isTargetLocal := r.Cfg.Matrix.IsLocalServerName(domain)
	if updater, err = r.DB.MembershipUpdater(ctx, inviteEvent.RoomID(), *inviteEvent.StateKey(), isTargetLocal, inviteEvent.Version()); err != nil {
		return nil, fmt.Errorf("r.DB.MembershipUpdater: %w", err)
	}
	outputUpdates, err = helpers.UpdateToInviteMembership(updater, &types.Event{
		EventNID: 0,
		PDU:      inviteEvent.PDU,
	}, outputUpdates, inviteEvent.Version())
	if err != nil {
		return nil, fmt.Errorf("updateToInviteMembership: %w", err)
	}
	if err = updater.Commit(); err != nil {
		return nil, fmt.Errorf("updater.Commit: %w", err)
	}
	return outputUpdates, nil
}

type QueryState struct {
	storage.Database
}

func (q *QueryState) GetAuthEvents(ctx context.Context, event gomatrixserverlib.PDU) (gomatrixserverlib.AuthEventProvider, error) {
	return helpers.GetAuthEvents(ctx, q.Database, event.Version(), event, event.AuthEventIDs())
}

// nolint:gocyclo
func (r *Inviter) PerformInvite(
	ctx context.Context,
	req *api.PerformInviteRequest,
) error {
	event := req.Event

	sender, err := spec.NewUserID(event.Sender(), true)
	if err != nil {
		return spec.InvalidParam("The user ID is invalid")
	}
	if !r.Cfg.Matrix.IsLocalServerName(sender.Domain()) {
		return api.ErrInvalidID{Err: fmt.Errorf("the invite must be from a local user")}
	}

	if event.StateKey() == nil {
		return fmt.Errorf("invite must be a state event")
	}
	invitedUser, err := spec.NewUserID(*event.StateKey(), true)
	if err != nil {
		return spec.InvalidParam("The user ID is invalid")
	}
	isTargetLocal := r.Cfg.Matrix.IsLocalServerName(invitedUser.Domain())

	validRoomID, err := spec.NewRoomID(event.RoomID())
	if err != nil {
		return err
	}

	input := PerformInviteInput{
		Context:               ctx,
		RoomID:                *validRoomID,
		Event:                 event.PDU,
		InvitedUser:           *invitedUser,
		IsTargetLocal:         isTargetLocal,
		StrippedState:         req.InviteRoomState,
		MembershipQuerier:     &api.MembershipQuerier{Roomserver: r.RSAPI},
		StateQuerier:          &QueryState{r.DB},
		GenerateStrippedState: r.GenerateInviteStrippedState,
	}
	inviteEvent, err := PerformInvite(input, r.FSAPI)
	if err != nil {
		return err
	}

	// Use the returned event if there was one (due to federation), otherwise
	// send the original invite event to the roomserver.
	if inviteEvent == nil {
		inviteEvent = event
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
				Event:        &types.HeaderedEvent{PDU: inviteEvent},
				Origin:       sender.Domain(),
				SendAsServer: req.SendAsServer,
			},
		},
	}
	inputRes := &api.InputRoomEventsResponse{}
	r.Inputer.InputRoomEvents(context.Background(), inputReq, inputRes)
	if err := inputRes.Err(); err != nil {
		util.GetLogger(ctx).WithField("event_id", event.EventID()).Error("r.InputRoomEvents failed")
		return api.ErrNotAllowed{Err: err}
	}

	return nil
}

// TODO: Move to gmsl

type StateQuerier interface {
	GetAuthEvents(ctx context.Context, event gomatrixserverlib.PDU) (gomatrixserverlib.AuthEventProvider, error)
}

type PerformInviteInput struct {
	Context               context.Context
	RoomID                spec.RoomID
	Event                 gomatrixserverlib.PDU
	InvitedUser           spec.UserID
	IsTargetLocal         bool
	StrippedState         []gomatrixserverlib.InviteStrippedState
	MembershipQuerier     gomatrixserverlib.MembershipQuerier
	StateQuerier          StateQuerier
	GenerateStrippedState func(ctx context.Context, roomID spec.RoomID, stateWanted []gomatrixserverlib.StateKeyTuple, inviteEvent gomatrixserverlib.PDU) ([]gomatrixserverlib.InviteStrippedState, error)
}

type FederatedInviteClient interface {
	SendInvite(ctx context.Context, event gomatrixserverlib.PDU, strippedState []gomatrixserverlib.InviteStrippedState) (gomatrixserverlib.PDU, error)
}

func PerformInvite(input PerformInviteInput, fedClient FederatedInviteClient) (gomatrixserverlib.PDU, error) {
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
		if is, generateErr := input.GenerateStrippedState(input.Context, input.RoomID, stateWanted, input.Event); generateErr == nil {
			inviteState = is
		} else {
			util.GetLogger(input.Context).WithError(generateErr).Error("failed querying known room")
			return nil, spec.InternalServerError{}
		}
	}

	logger := util.GetLogger(input.Context).WithFields(map[string]interface{}{
		"inviter":  input.Event.Sender(),
		"invitee":  *input.Event.StateKey(),
		"room_id":  input.RoomID.String(),
		"event_id": input.Event.EventID(),
	})
	logger.WithFields(log.Fields{
		"room_version": input.Event.Version(),
		"target_local": input.IsTargetLocal,
		"origin_local": true,
	}).Debug("processing invite event")

	if len(inviteState) == 0 {
		if err := input.Event.SetUnsignedField("invite_room_state", struct{}{}); err != nil {
			return nil, fmt.Errorf("event.SetUnsignedField: %w", err)
		}
	} else {
		if err := input.Event.SetUnsignedField("invite_room_state", inviteState); err != nil {
			return nil, fmt.Errorf("event.SetUnsignedField: %w", err)
		}
	}

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
		return nil, api.ErrNotAllowed{Err: fmt.Errorf("user is already joined to room")}
	}

	// The invite originated locally. Therefore we have a responsibility to
	// try and see if the user is allowed to make this invite. We can't do
	// this for invites coming in over federation - we have to take those on
	// trust.
	authEventProvider, err := input.StateQuerier.GetAuthEvents(input.Context, input.Event)
	if err != nil {
		logger.WithError(err).WithField("event_id", input.Event.EventID()).WithField("auth_event_ids", input.Event.AuthEventIDs()).Error(
			"ProcessInvite.getAuthEvents failed for event",
		)
		return nil, api.ErrNotAllowed{Err: err}
	}

	// Check if the event is allowed.
	if err = gomatrixserverlib.Allowed(input.Event, authEventProvider); err != nil {
		logger.WithError(err).WithField("event_id", input.Event.EventID()).WithField("auth_event_ids", input.Event.AuthEventIDs()).Error(
			"ProcessInvite: event not allowed",
		)
		return nil, api.ErrNotAllowed{Err: err}
	}

	// If the target isn't local then we should try and send the invite
	// over federation first. It might be that the remote user doesn't exist,
	// in which case we can give up processing here.
	var inviteEvent gomatrixserverlib.PDU
	if !input.IsTargetLocal {
		inviteEvent, err = fedClient.SendInvite(input.Context, input.Event, inviteState)
		if err != nil {
			logger.WithError(err).WithField("event_id", input.Event.EventID()).Error("fedClient.SendInvite failed")
			return nil, api.ErrNotAllowed{Err: err}
		}
		logger.Debugf("Federated SendInvite success with event ID %s", input.Event.EventID())
	}

	return inviteEvent, nil
}
