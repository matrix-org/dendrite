package input

import (
	"bytes"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// updateLatestEvents updates the list of latest events for this room in the database and writes the
// event to the output log.
// The latest events are the events that aren't referenced by another event in the database:
//
//     Time goes down the page. 1 is the m.room.create event (root).
//
//        1                 After storing 1 the latest events are {1}
//        |                 After storing 2 the latest events are {2}
//        2                 After storing 3 the latest events are {3}
//       / \                After storing 4 the latest events are {3,4}
//      3   4               After storing 5 the latest events are {5,4}
//      |   |               After storing 6 the latest events are {5,6}
//      5   6 <--- latest   After storing 7 the latest events are {6,7}
//      |
//      7 <----- latest
//
func updateLatestEvents(
	db RoomEventDatabase, ow OutputRoomEventWriter, roomNID types.RoomNID, stateAtEvent types.StateAtEvent, event gomatrixserverlib.Event,
) (err error) {
	oldLatest, lastEventIDSent, updater, err := db.GetLatestEventsForUpdate(roomNID)
	if err != nil {
		return
	}
	defer func() {
		if err == nil {
			// Commit if there wasn't an error.
			// Set the returned err value if we encounter an error committing.
			// This only works because err is a named return.
			err = updater.Commit()
		} else {
			// Ignore any error we get rolling back since we don't want to
			// clobber the current error
			// TODO: log the error here.
			updater.Rollback()
		}
	}()

	err = doUpdateLatestEvents(updater, ow, oldLatest, lastEventIDSent, roomNID, stateAtEvent, event)
	return
}

func doUpdateLatestEvents(
	updater types.RoomRecentEventsUpdater, ow OutputRoomEventWriter, oldLatest []types.StateAtEventAndReference, lastEventIDSent string, roomNID types.RoomNID, stateAtEvent types.StateAtEvent, event gomatrixserverlib.Event,
) error {
	var err error
	var prevEvents []gomatrixserverlib.EventReference
	prevEvents = event.PrevEvents()

	if hasBeenSent, err := updater.HasEventBeenSent(stateAtEvent.EventNID); err != nil {
		return err
	} else if hasBeenSent {
		// Already sent this event so we can stop processing
		return nil
	}

	if err = updater.StorePreviousEvents(stateAtEvent.EventNID, prevEvents); err != nil {
		return err
	}

	eventReference := event.EventReference()
	// Check if this event is already referenced by another event in the room.
	var alreadyReferenced bool
	if alreadyReferenced, err = updater.IsReferenced(eventReference); err != nil {
		return err
	}

	newLatest := calculateLatest(oldLatest, alreadyReferenced, prevEvents, types.StateAtEventAndReference{
		EventReference: eventReference,
		StateAtEvent:   stateAtEvent,
	})

	// Send the event to the output logs.
	// We do this inside the database transaction to ensure that we only mark an event as sent if we sent it.
	// TODO: This assumes that writing the event to the output log is synchronous. It should be possible to
	// send the event asynchronously but we would need to ensure that 1) the events are written to the log in
	// the correct order, 2) that pending writes are resent across restarts. In order to avoid writing all the
	// necessary bookkeeping we'll keep the event sending synchronous for now.
	if err = writeEvent(ow, lastEventIDSent, event, newLatest); err != nil {
		return err
	}

	if err = updater.SetLatestEvents(roomNID, newLatest, stateAtEvent.EventNID); err != nil {
		return err
	}

	if err = updater.MarkEventAsSent(stateAtEvent.EventNID); err != nil {
		return err
	}

	return nil
}

func calculateLatest(oldLatest []types.StateAtEventAndReference, alreadyReferenced bool, prevEvents []gomatrixserverlib.EventReference, newEvent types.StateAtEventAndReference) []types.StateAtEventAndReference {
	var alreadyInLatest bool
	var newLatest []types.StateAtEventAndReference
	for _, l := range oldLatest {
		keep := true
		for _, prevEvent := range prevEvents {
			if l.EventID == prevEvent.EventID && bytes.Compare(l.EventSHA256, prevEvent.EventSHA256) == 0 {
				// This event can be removed from the latest events cause we've found an event that references it.
				// (If an event is referenced by another event then it can't be one of the latest events in the room
				//  because we have an event that comes after it)
				keep = false
				break
			}
		}
		if l.EventNID == newEvent.EventNID {
			alreadyInLatest = true
		}
		if keep {
			// Keep the event in the latest events.
			newLatest = append(newLatest, l)
		}
	}

	if !alreadyReferenced && !alreadyInLatest {
		// This event is not referenced by any of the events in the room
		// and the event is not already in the latest events.
		// Add it to the latest events
		newLatest = append(newLatest, newEvent)
	}

	return newLatest
}

func writeEvent(ow OutputRoomEventWriter, lastEventIDSent string, event gomatrixserverlib.Event, latest []types.StateAtEventAndReference) error {

	latestEventIDs := make([]string, len(latest))
	for i := range latest {
		latestEventIDs[i] = latest[i].EventID
	}

	// TODO: Fill out AddsStateEventIDs and RemovesStateEventIDs
	// TODO: Fill out VisibilityStateIDs
	return ow.WriteOutputRoomEvent(api.OutputRoomEvent{
		Event:           event.JSON(),
		LastSentEventID: lastEventIDSent,
		LatestEventIDs:  latestEventIDs,
	})
}
