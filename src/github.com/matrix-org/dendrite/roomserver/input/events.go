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

package input

import (
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A RoomEventDatabase has the storage APIs needed to store a room event.
type RoomEventDatabase interface {
	state.RoomStateDatabase
	// Stores a matrix room event in the database
	StoreEvent(event gomatrixserverlib.Event, authEventNIDs []types.EventNID) (types.RoomNID, types.StateAtEvent, error)
	// Lookup the state entries for a list of string event IDs
	// Returns an error if the there is an error talking to the database
	// or if the event IDs aren't in the database.
	StateEntriesForEventIDs(eventIDs []string) ([]types.StateEntry, error)
	// Set the state at an event.
	SetState(eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	// Lookup the latest events in a room in preparation for an update.
	// The RoomRecentEventsUpdater must have Commit or Rollback called on it if this doesn't return an error.
	// Returns the latest events in the room and the last eventID sent to the log along with an updater.
	// If this returns an error then no further action is required.
	GetLatestEventsForUpdate(roomNID types.RoomNID) (updater types.RoomRecentEventsUpdater, err error)
	// Lookup the string event IDs for a list of numeric event IDs
	EventIDs(eventNIDs []types.EventNID) (map[types.EventNID]string, error)
}

// OutputRoomEventWriter has the APIs needed to write an event to the output logs.
type OutputRoomEventWriter interface {
	// Write an event.
	WriteOutputRoomEvent(output api.OutputRoomEvent) error
}

func processRoomEvent(db RoomEventDatabase, ow OutputRoomEventWriter, input api.InputRoomEvent) error {
	// Parse and validate the event JSON
	event, err := gomatrixserverlib.NewEventFromUntrustedJSON(input.Event)
	if err != nil {
		return err
	}

	// Check that the event passes authentication checks and work out the numeric IDs for the auth events.
	authEventNIDs, err := checkAuthEvents(db, event, input.AuthEventIDs)
	if err != nil {
		return err
	}

	// Store the event
	roomNID, stateAtEvent, err := db.StoreEvent(event, authEventNIDs)
	if err != nil {
		return err
	}

	if input.Kind == api.KindOutlier {
		// For outliers we can stop after we've stored the event itself as it
		// doesn't have any associated state to store and we don't need to
		// notify anyone about it.
		return nil
	}

	if stateAtEvent.BeforeStateSnapshotNID == 0 {
		// We haven't calculated a state for this event yet.
		// Lets calculate one.
		if input.HasState {
			// We've been told what the state at the event is so we don't need to calculate it.
			// Check that those state events are in the database and store the state.
			entries, err := db.StateEntriesForEventIDs(input.StateEventIDs)
			if err != nil {
				return err
			}

			if stateAtEvent.BeforeStateSnapshotNID, err = db.AddState(roomNID, nil, entries); err != nil {
				return nil
			}
		} else {
			// We haven't been told what the state at the event is so we need to calculate it from the prev_events
			if stateAtEvent.BeforeStateSnapshotNID, err = state.CalculateAndStoreStateBeforeEvent(db, event, roomNID); err != nil {
				return err
			}
		}
		db.SetState(stateAtEvent.EventNID, stateAtEvent.BeforeStateSnapshotNID)
	}

	if input.Kind == api.KindBackfill {
		// Backfill is not implemented.
		panic("Not implemented")
	}

	// Update the extremities of the event graph for the room
	if err := updateLatestEvents(db, ow, roomNID, stateAtEvent, event); err != nil {
		return err
	}

	// TODO:
	//  * Caculate the new current state for the room if the forward extremities have changed.
	//  * Work out the delta between the new current state and the previous current state.
	//  * Work out the visibility of the event.
	//  * Write a message to the output logs containing:
	//      - The event itself
	//      - The visiblity of the event, i.e. who is allowed to see the event.
	//      - The changes to the current state of the room.
	return nil
}
