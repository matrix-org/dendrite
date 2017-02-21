package input

import (
	"bytes"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// updateLatestEvents updates the list of latest events for this room.
func updateLatestEvents(
	db RoomEventDatabase, roomNID types.RoomNID, stateAtEvent types.StateAtEvent, event gomatrixserverlib.Event,
) (err error) {
	oldLatest, updater, err := db.GetLatestEventsForUpdate(roomNID)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			// Commit if there wasn't an error.
			// Set the returned err value if we encounter an error committing.
			err = updater.Close(true)
		} else {
			// Ignore any error we get rolling back since we don't want to
			// clobber the current error
			updater.Close(false)
		}
	}()

	var prevEvents []gomatrixserverlib.EventReference
	prevEvents = event.PrevEvents()

	if err = updater.StorePreviousEvents(stateAtEvent.EventNID, prevEvents); err != nil {
		return err
	}

	// Check if this event references any of the latest events in the room.
	var newLatest []types.StateAtEventAndReference
	for _, l := range oldLatest {
		for _, prevEvent := range prevEvents {
			if l.EventID == prevEvent.EventID && bytes.Compare(l.EventSHA256, prevEvent.EventSHA256) == 0 {
				// This event can be removed from the latest events cause we've found an event that references it.
				continue
			}
			// Keep the event in the latest events.
			newLatest = append(newLatest, l)
		}
	}

	eventReference := event.EventReference()
	// Check if this event is already referenced by another event in the room.
	var alreadyReferenced bool
	if alreadyReferenced, err = updater.IsReferenced(eventReference); err != nil {
		return err
	}

	if !alreadyReferenced {
		// This event is not referenced by any of the events in the room.
		// Add it to the latest events
		newLatest = append(newLatest, types.StateAtEventAndReference{
			StateAtEvent:   stateAtEvent,
			EventReference: eventReference,
		})
	}

	if err = updater.SetLatestEvents(roomNID, newLatest); err != nil {
		return err
	}

	// The err should be nil at this point.
	// But when we call Close in the defer above it might set an error here.
	return
}
