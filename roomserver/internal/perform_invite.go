package internal

import (
	"context"
	"errors"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// PerformInvite handles inviting to matrix rooms, including over federation by talking to the federationsender.
func (r *RoomserverInternalAPI) PerformInvite(
	ctx context.Context,
	req *api.PerformInviteRequest,
	res *api.PerformInviteResponse,
) {
	err := r.performInvite(ctx, req)
	if err != nil {
		perr, ok := err.(*api.PerformError)
		if ok {
			res.Error = perr
		} else {
			res.Error = &api.PerformError{
				Msg: err.Error(),
			}
		}
	}
}

func (r *RoomserverInternalAPI) performInvite(ctx context.Context,
	req *api.PerformInviteRequest,
) error {
	loopback, err := r.processInviteEvent(ctx, r, req)
	if err != nil {
		return err
	}
	// The processInviteEvent function can optionally return a
	// loopback room event containing the invite, for local invites.
	// If it does, we should process it with the room events below.
	if loopback != nil {
		var loopbackRes api.InputRoomEventsResponse
		err := r.InputRoomEvents(ctx, &api.InputRoomEventsRequest{
			InputRoomEvents: []api.InputRoomEvent{*loopback},
		}, &loopbackRes)
		if err != nil {
			return err
		}
	}
	return nil
}

// nolint:gocyclo
func (r *RoomserverInternalAPI) processInviteEvent(
	ctx context.Context,
	ow *RoomserverInternalAPI,
	input *api.PerformInviteRequest,
) (*api.InputRoomEvent, error) {
	if input.Event.StateKey() == nil {
		return nil, fmt.Errorf("invite must be a state event")
	}

	roomID := input.Event.RoomID()
	targetUserID := *input.Event.StateKey()

	log.WithFields(log.Fields{
		"event_id":       input.Event.EventID(),
		"room_id":        roomID,
		"room_version":   input.RoomVersion,
		"target_user_id": targetUserID,
	}).Info("processing invite event")

	_, domain, _ := gomatrixserverlib.SplitID('@', targetUserID)
	isTargetLocalUser := domain == r.Cfg.Matrix.ServerName

	updater, err := r.DB.MembershipUpdater(ctx, roomID, targetUserID, isTargetLocalUser, input.RoomVersion)
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		txerr := sqlutil.EndTransaction(updater, &succeeded)
		if err == nil && txerr != nil {
			err = txerr
		}
	}()

	if updater.IsJoin() {
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
		return nil, &api.PerformError{
			Code: api.PerformErrorNoOperation,
			Msg:  "user is already joined to room",
		}
	}

	// Normally, with a federated invite, the federation sender would do
	// the /v2/invite request (in which the remote server signs the invite)
	// and then the signed event gets sent back to the roomserver as an input
	// event. When the invite is local, we don't interact with the federation
	// sender therefore we need to generate the loopback invite event for
	// the room ourselves.
	loopback, err := localInviteLoopback(ow, input)
	if err != nil {
		return nil, err
	}

	event := input.Event.Unwrap()

	// check that the user is allowed to do this. We can only do this check if it is
	// a local invite as we have the auth events, else we have to take it on trust.
	if loopback != nil {
		_, err = checkAuthEvents(ctx, r.DB, input.Event, input.Event.AuthEventIDs())
		if err != nil {
			log.WithError(err).WithField("event_id", event.EventID()).WithField("auth_event_ids", event.AuthEventIDs()).Error(
				"processInviteEvent.checkAuthEvents failed for event",
			)
			if _, ok := err.(*gomatrixserverlib.NotAllowed); ok {
				return nil, &api.PerformError{
					Msg:  err.Error(),
					Code: api.PerformErrorNotAllowed,
				}
			}
			return nil, err
		}
	}

	if len(input.InviteRoomState) > 0 {
		// If we were supplied with some invite room state already (which is
		// most likely to be if the event came in over federation) then use
		// that.
		if err = event.SetUnsignedField("invite_room_state", input.InviteRoomState); err != nil {
			return nil, err
		}
	} else {
		// There's no invite room state, so let's have a go at building it
		// up from local data (which is most likely to be if the event came
		// from the CS API). If we know about the room then we can insert
		// the invite room state, if we don't then we just fail quietly.
		if irs, ierr := buildInviteStrippedState(ctx, r.DB, input); ierr == nil {
			if err = event.SetUnsignedField("invite_room_state", irs); err != nil {
				return nil, err
			}
		} else {
			log.WithError(ierr).Error("failed to build invite stripped state")
			// still set the field else synapse deployments don't process the invite
			if err = event.SetUnsignedField("invite_room_state", struct{}{}); err != nil {
				return nil, err
			}
		}
	}

	outputUpdates, err := updateToInviteMembership(updater, &event, nil, input.Event.RoomVersion)
	if err != nil {
		return nil, err
	}

	if err = ow.WriteOutputEvents(roomID, outputUpdates); err != nil {
		return nil, err
	}

	succeeded = true
	return loopback, nil
}

func localInviteLoopback(
	ow *RoomserverInternalAPI,
	input *api.PerformInviteRequest,
) (ire *api.InputRoomEvent, err error) {
	if input.Event.StateKey() == nil {
		return nil, errors.New("no state key on invite event")
	}
	ourServerName := string(ow.Cfg.Matrix.ServerName)
	_, theirServerName, err := gomatrixserverlib.SplitID('@', *input.Event.StateKey())
	if err != nil {
		return nil, err
	}
	// Check if the invite originated locally and is destined locally.
	if input.Event.Origin() == ow.Cfg.Matrix.ServerName && string(theirServerName) == ourServerName {
		rsEvent := input.Event.Sign(
			ourServerName,
			ow.Cfg.Matrix.KeyID,
			ow.Cfg.Matrix.PrivateKey,
		).Headered(input.RoomVersion)
		ire = &api.InputRoomEvent{
			Kind:          api.KindNew,
			Event:         rsEvent,
			AuthEventIDs:  rsEvent.AuthEventIDs(),
			SendAsServer:  ourServerName,
			TransactionID: nil,
		}
	}
	return ire, nil
}

func buildInviteStrippedState(
	ctx context.Context,
	db storage.Database,
	input *api.PerformInviteRequest,
) ([]gomatrixserverlib.InviteV2StrippedState, error) {
	roomNID, err := db.RoomNID(ctx, input.Event.RoomID())
	if err != nil || roomNID == 0 {
		return nil, fmt.Errorf("room %q unknown", input.Event.RoomID())
	}
	stateWanted := []gomatrixserverlib.StateKeyTuple{}
	// "If they are set on the room, at least the state for m.room.avatar, m.room.canonical_alias, m.room.join_rules, and m.room.name SHOULD be included."
	// https://matrix.org/docs/spec/client_server/r0.6.0#m-room-member
	for _, t := range []string{
		gomatrixserverlib.MRoomName, gomatrixserverlib.MRoomCanonicalAlias,
		gomatrixserverlib.MRoomAliases, gomatrixserverlib.MRoomJoinRules,
		"m.room.avatar",
	} {
		stateWanted = append(stateWanted, gomatrixserverlib.StateKeyTuple{
			EventType: t,
			StateKey:  "",
		})
	}
	_, currentStateSnapshotNID, _, err := db.LatestEventIDs(ctx, roomNID)
	if err != nil {
		return nil, err
	}
	roomState := state.NewStateResolution(db)
	stateEntries, err := roomState.LoadStateAtSnapshotForStringTuples(
		ctx, currentStateSnapshotNID, stateWanted,
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
		gomatrixserverlib.NewInviteV2StrippedState(&input.Event.Event),
	}
	stateEvents = append(stateEvents, types.Event{Event: input.Event.Unwrap()})
	for _, event := range stateEvents {
		inviteState = append(inviteState, gomatrixserverlib.NewInviteV2StrippedState(&event.Event))
	}
	return inviteState, nil
}
