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
	event, err := gomatrixserverlib.NewEventFromUntrustedJSON(input.Event)
	if err != nil {
		return err
	}

	if err := db.StoreEvent(event); err != nil {
		return err
	}

	if input.Kind == api.KindOutlier {
		// For outlier events we only need to store the event JSON.
		return nil
	}

	// TODO: Handle the other kinds of input.
	panic("Not implemented")
}
