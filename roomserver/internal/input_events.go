// Copyright 2017 Vector Creations Ltd
// Copyright 2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

// processRoomEvent can only be called once at a time
//
// TODO(#375): This should be rewritten to allow concurrent calls. The
// difficulty is in ensuring that we correctly annotate events with the correct
// state deltas when sending to kafka streams
func (r *RoomserverInternalAPI) processRoomEvent(
	ctx context.Context,
	input api.InputRoomEvent,
) (eventID string, err error) {
	// Parse and validate the event JSON
	headered := input.Event
	event := headered.Unwrap()

	// Check that the event passes authentication checks and work out
	// the numeric IDs for the auth events.
	authEventNIDs, err := checkAuthEvents(ctx, r.DB, headered, input.AuthEventIDs)
	if err != nil {
		logrus.WithError(err).WithField("event_id", event.EventID()).WithField("auth_event_ids", input.AuthEventIDs).Error("processRoomEvent.checkAuthEvents failed for event")
		return
	}

	// If we don't have a transaction ID then get one.
	if input.TransactionID != nil {
		tdID := input.TransactionID
		eventID, err = r.DB.GetTransactionEventID(
			ctx, tdID.TransactionID, tdID.SessionID, event.Sender(),
		)
		// On error OR event with the transaction already processed/processesing
		if err != nil || eventID != "" {
			return
		}
	}

	// Store the event.
	roomNID, stateAtEvent, err := r.DB.StoreEvent(ctx, event, input.TransactionID, authEventNIDs)
	if err != nil {
		return
	}

	// For outliers we can stop after we've stored the event itself as it
	// doesn't have any associated state to store and we don't need to
	// notify anyone about it.
	if input.Kind == api.KindOutlier {
		logrus.WithFields(logrus.Fields{
			"event_id": event.EventID(),
			"type":     event.Type(),
			"room":     event.RoomID(),
		}).Info("Stored outlier")
		return event.EventID(), nil
	}

	if stateAtEvent.BeforeStateSnapshotNID == 0 {
		// We haven't calculated a state for this event yet.
		// Lets calculate one.
		err = r.calculateAndSetState(ctx, input, roomNID, &stateAtEvent, event)
		if err != nil {
			return
		}
	}

	if err = r.updateLatestEvents(
		ctx,                 // context
		roomNID,             // room NID to update
		stateAtEvent,        // state at event (below)
		event,               // event
		input.SendAsServer,  // send as server
		input.TransactionID, // transaction ID
	); err != nil {
		return
	}

	// Update the extremities of the event graph for the room
	return event.EventID(), nil
}

func (r *RoomserverInternalAPI) calculateAndSetState(
	ctx context.Context,
	input api.InputRoomEvent,
	roomNID types.RoomNID,
	stateAtEvent *types.StateAtEvent,
	event gomatrixserverlib.Event,
) error {
	var err error
	roomState := state.NewStateResolution(r.DB)

	if input.HasState {
		// Check here if we think we're in the room already.
		stateAtEvent.Overwrite = true
		var joinEventNIDs []types.EventNID
		// Request join memberships only for local users only.
		if joinEventNIDs, err = r.DB.GetMembershipEventNIDsForRoom(ctx, roomNID, true, true); err == nil {
			// If we have no local users that are joined to the room then any state about
			// the room that we have is quite possibly out of date. Therefore in that case
			// we should overwrite it rather than merge it.
			stateAtEvent.Overwrite = len(joinEventNIDs) == 0
		}

		// We've been told what the state at the event is so we don't need to calculate it.
		// Check that those state events are in the database and store the state.
		var entries []types.StateEntry
		if entries, err = r.DB.StateEntriesForEventIDs(ctx, input.StateEventIDs); err != nil {
			return err
		}

		if stateAtEvent.BeforeStateSnapshotNID, err = r.DB.AddState(ctx, roomNID, nil, entries); err != nil {
			return err
		}
	} else {
		stateAtEvent.Overwrite = false

		// We haven't been told what the state at the event is so we need to calculate it from the prev_events
		if stateAtEvent.BeforeStateSnapshotNID, err = roomState.CalculateAndStoreStateBeforeEvent(ctx, event, roomNID); err != nil {
			return err
		}
	}
	return r.DB.SetState(ctx, stateAtEvent.EventNID, stateAtEvent.BeforeStateSnapshotNID)
}

func (r *RoomserverInternalAPI) processInviteEvent(
	ctx context.Context,
	ow *RoomserverInternalAPI,
	input api.InputInviteEvent,
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
		return nil, nil
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
	input api.InputInviteEvent,
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
	input api.InputInviteEvent,
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
