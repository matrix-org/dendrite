package input

import (
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// A RoomEventDatabase has the storage APIs needed to store a room event.
type RoomEventDatabase interface {
	StoreEvent(event gomatrixserverlib.Event) error
}

func processRoomEvent(db RoomEventDatabase, input api.InputRoomEvent) error {
	// Parse and validate the event JSON
	event, err := gomatrixserverlib.NewEventFromUntrustedJSON(input.Event)
	if err != nil {
		return err
	}

	if err := db.StoreEvent(event); err != nil {
		return err
	}

	// TODO:
	//  * Check that the event passes authentication checks.

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
