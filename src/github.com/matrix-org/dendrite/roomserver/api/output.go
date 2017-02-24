package api

import (
	"encoding/json"
)

// An OutputRoomEvent is written when the roomserver receives a new event.
type OutputRoomEvent struct {
	// The JSON bytes of the event.
	Event []byte
	// The state event IDs needed to determine who can see this event.
	// This can be used to tell which users to send the event to.
	VisibilityEventIDs []string
	// The latest events in the room after this event.
	// This can be used to set the prev events for new events in the room.
	// This also can be used to get the full current state after this event.
	LatestEventIDs []string
	// The state event IDs that were added to the state of the room by this event.
	// Together with RemovesStateEventIDs this allows the receiver to keep an up to date
	// view of the current state of the room.
	AddsStateEventIDs []string
	// The state event IDs that were removed from the state of the room by this event.
	RemovesStateEventIDs []string
	// The ID of the event that was output before this event.
	// Or the empty string if this is the first event output for this room.
	// This is used by consumers to check if they can safely update their
	// current state using the delta supplied in AddsStateEventIDs and
	// RemovesStateEventIDs.
	// If they the LastSentEventID doesn't match what they were expecting it to
	// be they can use the LatestEventIDs to request the full current state.
	LastSentEventID string
}

// UnmarshalJSON implements json.Unmarshaller
func (ore *OutputRoomEvent) UnmarshalJSON(data []byte) error {
	// Create a struct rather than unmarshalling directly into the OutputRoomEvent
	// so that we can use json.RawMessage.
	// We use json.RawMessage so that the event JSON is sent as JSON rather than
	// being base64 encoded which is the default for []byte.
	var content struct {
		Event                *json.RawMessage
		VisibilityEventIDs   []string
		LatestEventIDs       []string
		AddsStateEventIDs    []string
		RemovesStateEventIDs []string
		LastSentEventID      string
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return err
	}
	if content.Event != nil {
		ore.Event = []byte(*content.Event)
	}
	ore.VisibilityEventIDs = content.VisibilityEventIDs
	ore.LatestEventIDs = content.LatestEventIDs
	ore.AddsStateEventIDs = content.AddsStateEventIDs
	ore.RemovesStateEventIDs = content.RemovesStateEventIDs
	ore.LastSentEventID = content.LastSentEventID
	return nil
}

// MarshalJSON implements json.Marshaller
func (ore OutputRoomEvent) MarshalJSON() ([]byte, error) {
	// Create a struct rather than marshalling directly from the OutputRoomEvent
	// so that we can use json.RawMessage.
	// We use json.RawMessage so that the event JSON is sent as JSON rather than
	// being base64 encoded which is the default for []byte.
	event := json.RawMessage(ore.Event)
	content := struct {
		Event                *json.RawMessage
		VisibilityEventIDs   []string
		LatestEventIDs       []string
		AddsStateEventIDs    []string
		RemovesStateEventIDs []string
		LastSentEventID      string
	}{
		Event:                &event,
		VisibilityEventIDs:   ore.VisibilityEventIDs,
		LatestEventIDs:       ore.LatestEventIDs,
		AddsStateEventIDs:    ore.AddsStateEventIDs,
		RemovesStateEventIDs: ore.RemovesStateEventIDs,
		LastSentEventID:      ore.LastSentEventID,
	}
	return json.Marshal(&content)
}
