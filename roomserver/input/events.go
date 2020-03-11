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

package input

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/state/database"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A RoomEventDatabase has the storage APIs needed to store a room event.
type RoomEventDatabase interface {
	database.RoomStateDatabase
	// Stores a matrix room event in the database
	StoreEvent(
		ctx context.Context,
		event gomatrixserverlib.Event,
		txnAndSessionID *api.TransactionID,
		authEventNIDs []types.EventNID,
	) (types.RoomNID, types.StateAtEvent, error)
	// Look up the state entries for a list of string event IDs
	// Returns an error if the there is an error talking to the database
	// Returns a types.MissingEventError if the event IDs aren't in the database.
	StateEntriesForEventIDs(
		ctx context.Context, eventIDs []string,
	) ([]types.StateEntry, error)
	// Set the state at an event.
	SetState(
		ctx context.Context,
		eventNID types.EventNID,
		stateNID types.StateSnapshotNID,
	) error
	// Look up the latest events in a room in preparation for an update.
	// The RoomRecentEventsUpdater must have Commit or Rollback called on it if this doesn't return an error.
	// Returns the latest events in the room and the last eventID sent to the log along with an updater.
	// If this returns an error then no further action is required.
	GetLatestEventsForUpdate(
		ctx context.Context, roomNID types.RoomNID,
	) (updater types.RoomRecentEventsUpdater, err error)
	// Look up the string event IDs for a list of numeric event IDs
	EventIDs(
		ctx context.Context, eventNIDs []types.EventNID,
	) (map[types.EventNID]string, error)
	// Build a membership updater for the target user in a room.
	MembershipUpdater(
		ctx context.Context, roomID, targerUserID string,
	) (types.MembershipUpdater, error)
	// Look up event ID by transaction's info.
	// This is used to determine if the room event is processed/processing already.
	// Returns an empty string if no such event exists.
	GetTransactionEventID(
		ctx context.Context, transactionID string,
		sessionID int64, userID string,
	) (string, error)
	// Look up the room version from the database.
	GetRoomVersionForRoomID(
		ctx context.Context, roomID string,
	) (gomatrixserverlib.RoomVersion, error)
	GetRoomVersionForRoomNID(
		ctx context.Context, roomNID types.RoomNID,
	) (gomatrixserverlib.RoomVersion, error)
}

// OutputRoomEventWriter has the APIs needed to write an event to the output logs.
type OutputRoomEventWriter interface {
	// Write a list of events for a room
	WriteOutputEvents(roomID string, roomVersion gomatrixserverlib.RoomVersion, updates []api.OutputEvent) error
}

// processRoomEvent can only be called once at a time
//
// TODO(#375): This should be rewritten to allow concurrent calls. The
// difficulty is in ensuring that we correctly annotate events with the correct
// state deltas when sending to kafka streams
func processRoomEvent(
	ctx context.Context,
	db RoomEventDatabase,
	ow OutputRoomEventWriter,
	input api.InputRoomEvent,
) (eventID string, err error) {
	// Parse and validate the event JSON
	event := input.Event

	// Check that the event passes authentication checks and work out the numeric IDs for the auth events.
	authEventNIDs, err := checkAuthEvents(ctx, db, event, input.AuthEventIDs)
	if err != nil {
		return
	}

	if input.TransactionID != nil {
		tdID := input.TransactionID
		eventID, err = db.GetTransactionEventID(
			ctx, tdID.TransactionID, tdID.SessionID, input.Event.Sender(),
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

	if input.Kind == api.KindBackfill {
		// Backfill is not implemented.
		panic("Not implemented")
	}

	// Update the extremities of the event graph for the room
	return event.EventID(), updateLatestEvents(
		ctx, db, ow, roomNID, stateAtEvent, event, input.SendAsServer, input.TransactionID,
	)
}

func calculateAndSetState(
	ctx context.Context,
	db RoomEventDatabase,
	input api.InputRoomEvent,
	roomNID types.RoomNID,
	stateAtEvent *types.StateAtEvent,
	event gomatrixserverlib.Event,
) error {
	roomVersion, err := db.GetRoomVersionForRoomNID(ctx, roomNID)
	if err != nil {
		return err
	}
	roomState, err := state.GetStateResolutionAlgorithm(roomVersion, db)
	if err != nil {
		return err
	}

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
	db RoomEventDatabase,
	ow OutputRoomEventWriter,
	input api.InputInviteEvent,
) (err error) {
	if input.Event.StateKey() == nil {
		return fmt.Errorf("invite must be a state event")
	}

	roomID := input.Event.RoomID()
	roomVersion := gomatrixserverlib.RoomVersionV1 // TODO: Feeeeeeex
	targetUserID := *input.Event.StateKey()

	updater, err := db.MembershipUpdater(ctx, roomID, targetUserID)
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

	outputUpdates, err := updateToInviteMembership(updater, &input.Event, nil)
	if err != nil {
		return err
	}

	if err = ow.WriteOutputEvents(roomID, roomVersion, outputUpdates); err != nil {
		return err
	}

	succeeded = true
	return nil
}
