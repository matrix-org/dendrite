package helpers

import (
	"encoding/json"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib"
)

// SanityCheckEvent looks for any obvious problems with the event before
// we bother to continue processing it any further.
func SanityCheckEvent(event *gomatrixserverlib.Event) error {
	switch event.Type() {
	case gomatrixserverlib.MRoomCreate:
		var content gomatrixserverlib.CreateContent
		if err := json.Unmarshal(event.Content(), &content); err != nil {
			return fmt.Errorf("Failed to unmarshal content of create event %q", event.EventID())
		}

		// Check that the room version is supported.
		if content.RoomVersion != nil {
			if _, err := content.RoomVersion.EventFormat(); err != nil {
				return fmt.Errorf("Room version %q is unsupported in create event %q", *content.RoomVersion, event.EventID())
			}
		}
	}
	return nil
}
