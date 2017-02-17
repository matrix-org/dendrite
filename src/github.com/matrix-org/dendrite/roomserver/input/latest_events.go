package input

import (
	"bytes"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

func updateLatestEvents(
	db RoomEventDatabase, roomNID types.RoomNID, stateAtEvent types.StateAtEvent, event gomatrixserverlib.Event,
) error {
	oldLatest, updater, err := db.GetLatestEventsForUpdate(roomNID)
	if err != nil {
		return err
	}
	defer func() {
		// Commit if there wasn't an error.
		updater.Close(err == nil)
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

	err = updater.SetLatestEvents(roomNID, newLatest)
	if err != nil {
		return err
	}

	// The deferred fires and the transaction closes.
	return nil
}
