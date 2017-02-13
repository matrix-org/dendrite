package input

import (
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A RoomEventDatabase has the storage APIs needed to store a room event.
type RoomEventDatabase interface {
	// Stores a matrix room event in the database
	StoreEvent(event gomatrixserverlib.Event, authEventNIDs []types.EventNID) (types.RoomNID, types.StateAtEvent, error)
	// Lookup the state entries for a list of string event IDs
	// Returns a sorted list of state entries.
	// Returns an error if the there is an error talking to the database
	// or if the event IDs aren't in the database.
	StateEntriesForEventIDs(eventIDs []string) ([]types.StateEntry, error)
	// Lookup the numeric IDs for a list of string event state keys.
	// Returns a map from string state key to numeric ID for the state key.
	EventStateKeyNIDs(eventStateKeys []string) (map[string]types.EventStateKeyNID, error)
	// Lookup the Events for a list of numeric event IDs.
	// Returns a sorted list of events.
	Events(eventNIDs []types.EventNID) ([]types.Event, error)
	// Lookup the state of a room at each event for a list of string event IDs.
	// Returns a sorted list of state at each event.
	// Returns an error if there is an error talking to the database
	// or if the room state for the event IDs aren't in the database
	StateAtEventIDs(eventIDs []string) ([]types.StateAtEvent, error)
	// Lookup the numeric state data IDs for the each numeric state ID
	// The returned slice is sorted by numeric state ID.
	StateDataNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateDataNIDList, error)
	// Lookup the state data for each numeric state data ID
	// The returned slice is sorted by numeric state data ID.
	StateEntries(stateDataNIDs []types.StateDataNID) ([]types.StateEntryList, error)
	// Store the room state at an event in the database
	AddState(roomNID types.RoomNID, stateDataNIDs []types.StateDataNID, state []types.StateEntry) (types.StateSnapshotNID, error)
	// Set the state at an event.
	SetState(eventNID types.EventNID, stateNID types.StateSnapshotNID) error
}

func processRoomEvent(db RoomEventDatabase, input api.InputRoomEvent) error {
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
		if stateAtEvent.BeforeStateSnapshotNID, err = calculateAndStoreState(db, event, roomNID, input.StateEventIDs); err != nil {
			return err
		}
		db.SetState(stateAtEvent.EventNID, stateAtEvent.BeforeStateSnapshotNID)
	}

	// TODO:
	//  * Calcuate the state at the event if necessary.
	//  * Store the state at the event.
	//  * Update the extremities of the event graph for the room
	//  * Caculate the new current state for the room if the forward extremities have changed.
	//  * Work out the delta between the new current state and the previous current state.
	//  * Work out the visibility of the event.
	//  * Write a message to the output logs containing:
	//      - The event itself
	//      - The visiblity of the event, i.e. who is allowed to see the event.
	//      - The changes to the current state of the room.
	panic("Not implemented")
}
