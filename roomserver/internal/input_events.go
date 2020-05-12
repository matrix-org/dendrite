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
	"fmt"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

// OutputRoomEventWriter has the APIs needed to write an event to the output logs.
type OutputRoomEventWriter interface {
	// Write a list of events for a room
	WriteOutputEvents(roomID string, updates []api.OutputEvent) error
}

// processRoomEvent can only be called once at a time
//
// TODO(#375): This should be rewritten to allow concurrent calls. The
// difficulty is in ensuring that we correctly annotate events with the correct
// state deltas when sending to kafka streams
func processRoomEvent(
	ctx context.Context,
	db storage.Database,
	ow OutputRoomEventWriter,
	input api.InputRoomEvent,
) (eventID string, err error) {
	// Parse and validate the event JSON
	headered := input.Event
	event := headered.Unwrap()

	// Check that the event passes authentication checks and work out the numeric IDs for the auth events.
	authEventNIDs, err := checkAuthEvents(ctx, db, headered, input.AuthEventIDs)
	if err != nil {
		logrus.WithError(err).WithField("event_id", event.EventID()).WithField("auth_event_ids", input.AuthEventIDs).Error("processRoomEvent.checkAuthEvents failed for event")
		return
	}

	if input.TransactionID != nil {
		tdID := input.TransactionID
		eventID, err = db.GetTransactionEventID(
			ctx, tdID.TransactionID, tdID.SessionID, event.Sender(),
		)
		// On error OR event with the transaction already processed/processesing
		if err != nil || eventID != "" {
			return
		}
	}

	// Store the event
	roomNID, stateAtEvent, err := db.StoreEvent(ctx, event, input.TransactionID, authEventNIDs)
	if err != nil {
		return
	}

	if input.Kind == api.KindOutlier {
		// For outliers we can stop after we've stored the event itself as it
		// doesn't have any associated state to store and we don't need to
		// notify anyone about it.
		logrus.WithField("event_id", event.EventID()).WithField("type", event.Type()).WithField("room", event.RoomID()).Info("Stored outlier")
		return event.EventID(), nil
	}

	if stateAtEvent.BeforeStateSnapshotNID == 0 {
		// We haven't calculated a state for this event yet.
		// Lets calculate one.
		err = calculateAndSetState(ctx, db, input, roomNID, &stateAtEvent, event)
		if err != nil {
			return
		}
	}

	// Update the extremities of the event graph for the room
	return event.EventID(), updateLatestEvents(
		ctx, db, ow, roomNID, stateAtEvent, event, input.SendAsServer, input.TransactionID,
	)
}

func calculateAndSetState(
	ctx context.Context,
	db storage.Database,
	input api.InputRoomEvent,
	roomNID types.RoomNID,
	stateAtEvent *types.StateAtEvent,
	event gomatrixserverlib.Event,
) error {
	var err error
	roomState := state.NewStateResolution(db)

	if input.HasState {
		// We've been told what the state at the event is so we don't need to calculate it.
		// Check that those state events are in the database and store the state.
		var entries []types.StateEntry
		if entries, err = db.StateEntriesForEventIDs(ctx, input.StateEventIDs); err != nil {
			return err
		}

		if stateAtEvent.BeforeStateSnapshotNID, err = db.AddState(ctx, roomNID, nil, entries); err != nil {
			return err
		}
	} else {
		// We haven't been told what the state at the event is so we need to calculate it from the prev_events
		if stateAtEvent.BeforeStateSnapshotNID, err = roomState.CalculateAndStoreStateBeforeEvent(ctx, event, roomNID); err != nil {
			return err
		}
	}
	return db.SetState(ctx, stateAtEvent.EventNID, stateAtEvent.BeforeStateSnapshotNID)
}

func processInviteEvent(
	ctx context.Context,
	db storage.Database,
	ow OutputRoomEventWriter,
	input api.InputInviteEvent,
) (err error) {
	if input.Event.StateKey() == nil {
		return fmt.Errorf("invite must be a state event")
	}

	roomID := input.Event.RoomID()
	targetUserID := *input.Event.StateKey()

	log.WithFields(log.Fields{
		"event_id":       input.Event.EventID(),
		"room_id":        roomID,
		"room_version":   input.RoomVersion,
		"target_user_id": targetUserID,
	}).Info("processing invite event")

	updater, err := db.MembershipUpdater(ctx, roomID, targetUserID, input.RoomVersion)
	if err != nil {
		return err
	}
	succeeded := false
	defer func() {
		txerr := common.EndTransaction(updater, &succeeded)
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
		return nil
	}

	event := input.Event.Unwrap()

	if len(input.InviteRoomState) > 0 {
		// If we were supplied with some invite room state already (which is
		// most likely to be if the event came in over federation) then use
		// that.
		if err = event.SetUnsignedField("invite_room_state", input.InviteRoomState); err != nil {
			return err
		}
	} else {
		// There's no invite room state, so let's have a go at building it
		// up from local data (which is most likely to be if the event came
		// from the CS API). If we know about the room then we can insert
		// the invite room state, if we don't then we just fail quietly.
		if irs, ierr := buildInviteStrippedState(ctx, db, input); ierr == nil {
			if err = event.SetUnsignedField("invite_room_state", irs); err != nil {
				return err
			}
		}
	}

	outputUpdates, err := updateToInviteMembership(updater, &event, nil, input.Event.RoomVersion)
	if err != nil {
		return err
	}

	if err = ow.WriteOutputEvents(roomID, outputUpdates); err != nil {
		return err
	}

	succeeded = true
	return nil
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
	for _, t := range []string{
		gomatrixserverlib.MRoomName, gomatrixserverlib.MRoomCanonicalAlias,
		gomatrixserverlib.MRoomAliases, gomatrixserverlib.MRoomJoinRules,
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
	for _, event := range stateEvents {
		inviteState = append(inviteState, gomatrixserverlib.NewInviteV2StrippedState(&event.Event))
	}
	return inviteState, nil
}
