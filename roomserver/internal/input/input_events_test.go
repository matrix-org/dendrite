package input

import (
	"testing"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/test"
)

func Test_EventAuth(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)

	// create two rooms, so we can craft "illegal" auth events
	room1 := test.NewRoom(t, alice)
	room2 := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat))

	authEventIDs := make([]string, 0, 4)
	authEvents := []*gomatrixserverlib.Event{}

	// Add the legal auth events from room2
	for _, x := range room2.Events() {
		if x.Type() == gomatrixserverlib.MRoomCreate {
			authEventIDs = append(authEventIDs, x.EventID())
			authEvents = append(authEvents, x.Event)
		}
		if x.Type() == gomatrixserverlib.MRoomPowerLevels {
			authEventIDs = append(authEventIDs, x.EventID())
			authEvents = append(authEvents, x.Event)
		}
		if x.Type() == gomatrixserverlib.MRoomJoinRules {
			authEventIDs = append(authEventIDs, x.EventID())
			authEvents = append(authEvents, x.Event)
		}
	}

	// Add the illegal auth event from room1 (rooms are different)
	for _, x := range room1.Events() {
		if x.Type() == gomatrixserverlib.MRoomMember {
			authEventIDs = append(authEventIDs, x.EventID())
			authEvents = append(authEvents, x.Event)
		}
	}

	// Craft the illegal join event, with auth events from different rooms
	ev := room2.CreateEvent(t, bob, "m.room.member", map[string]interface{}{
		"membership": "join",
	}, test.WithStateKey(bob.ID), test.WithAuthIDs(authEventIDs))

	// Add the auth events to the allower
	allower := gomatrixserverlib.NewAuthEvents(nil)
	for _, a := range authEvents {
		if err := allower.AddEvent(a); err != nil {
			t.Fatalf("allower.AddEvent failed: %v", err)
		}
	}

	// Finally check that the event is NOT allowed
	if err := gomatrixserverlib.Allowed(ev.Event, &allower); err == nil {
		t.Fatalf("event should not be allowed, but it was")
	}
}
