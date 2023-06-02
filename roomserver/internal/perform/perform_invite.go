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
)

type QueryState struct {
	storage.Database
}

func (q *QueryState) GetAuthEvents(ctx context.Context, event gomatrixserverlib.PDU) (gomatrixserverlib.AuthEventProvider, error) {
	return helpers.GetAuthEvents(ctx, q.Database, event.Version(), event, event.AuthEventIDs())
}

func (q *QueryState) GetState(ctx context.Context, roomID spec.RoomID, stateWanted []gomatrixserverlib.StateKeyTuple) ([]gomatrixserverlib.PDU, error) {
	info, err := q.Database.RoomInfo(ctx, roomID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to load RoomInfo: %w", err)
	}
	if info != nil {
		roomState := state.NewStateResolution(q.Database, info)
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
		stateEvents, err := q.Database.Events(ctx, info.RoomVersion, stateNIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to obtain required events: %w", err)
		}

		events := []gomatrixserverlib.PDU{}
		for _, event := range stateEvents {
			events = append(events, event.PDU)
		}
		return events, nil
	}

	return nil, nil
}

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

func (r *Inviter) StateQuerier() gomatrixserverlib.StateQuerier {
	return &QueryState{Database: r.DB}
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

// nolint:gocyclo
func (r *Inviter) PerformInvite(
	ctx context.Context,
	req *api.PerformInviteRequest,
) error {
	event := req.Event

	sender, err := event.UserID()
	if err != nil {
		return spec.InvalidParam("The sender user ID is invalid")
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

	input := gomatrixserverlib.PerformInviteInput{
		RoomID:            *validRoomID,
		InviteEvent:       event.PDU,
		InvitedUser:       *invitedUser,
		IsTargetLocal:     isTargetLocal,
		StrippedState:     req.InviteRoomState,
		MembershipQuerier: &api.MembershipQuerier{Roomserver: r.RSAPI},
		StateQuerier:      &QueryState{r.DB},
	}
	inviteEvent, err := gomatrixserverlib.PerformInvite(ctx, input, r.FSAPI)
	if err != nil {
		switch e := err.(type) {
		case spec.MatrixError:
			if e.ErrCode == spec.ErrorForbidden {
				return api.ErrNotAllowed{Err: fmt.Errorf("%s", e.Err)}
			}
		}
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
