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
	// Returns an error if there is an error talking to the database
	// or if the room state for the event IDs aren't in the database
	StateAtEventIDs(eventIDs []string) ([]types.StateAtEvent, error)
	// Lookup the numeric state data IDs for each numeric state snapshot ID
	// The returned slice is sorted by numeric state snapshot ID.
	StateBlockNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error)
	// Lookup the state data for each numeric state data ID
	// The returned slice is sorted by numeric state data ID.
	StateEntries(stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error)
	// Store the room state at an event in the database
	AddState(roomNID types.RoomNID, stateBlockNIDs []types.StateBlockNID, state []types.StateEntry) (types.StateSnapshotNID, error)
	// Set the state at an event.
	SetState(eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	// Lookup the latest events in a room in preparation for an update.
	// The RoomRecentEventsUpdater must have Commit or Rollback called on it if this doesn't return an error.
	// Returns the latest events in the room and the last eventID sent to the log along with an updater.
	// If this returns an error then no further action is required.
	GetLatestEventsForUpdate(roomNID types.RoomNID) (
		latestEvents []types.StateAtEventAndReference, lastEventIDSent string, updater types.RoomRecentEventsUpdater, err error,
	)
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
			if stateAtEvent.BeforeStateSnapshotNID, err = calculateAndStoreState(db, event, roomNID); err != nil {
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
