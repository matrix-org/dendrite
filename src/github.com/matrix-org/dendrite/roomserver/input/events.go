package input

import (
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A RoomEventDatabase has the storage APIs needed to store a room event.
type RoomEventDatabase interface {
	// Stores a matrix room event in the database
	StoreEvent(event gomatrixserverlib.Event, authEventNIDs []int64) error
	// Lookup the state entries for a list of string event IDs
	// Returns a sorted list of state entries.
	// Returns a error if the there is an error talking to the database
	// or if the event IDs aren't in the database.
	StateEntriesForEventIDs(eventIDs []string) ([]types.StateEntry, error)
	// Lookup the numeric IDs for a list of string event state keys.
	// Returns a map from string state key to numeric ID for the state key.
	EventStateKeyNIDs(eventStateKeys []string) (map[string]int64, error)
	// Lookup the Events for a list of numeric event IDs.
	// Returns a sorted list of events.
	Events(eventNIDs []int64) ([]types.Event, error)
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
	if err := db.StoreEvent(event, authEventNIDs); err != nil {
		return err
	}

	if input.Kind == api.KindOutlier {
		// For outliers we can stop after we've stored the event itself as it
		// doesn't have any associated state to store and we don't need to
		// notify anyone about it.
		return nil
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
