// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"encoding/json"
)

// An OutputRoomEvent is written when the roomserver receives a new event.
type OutputRoomEvent struct {
	// The JSON bytes of the event.
	Event []byte
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
	// If the LastSentEventID doesn't match what they were expecting it to be
	// they can use the LatestEventIDs to request the full current state.
	LastSentEventID string
	// The state event IDs that are part of the state at the event, but not
	// part of the current state. Together with the StateBeforeRemovesEventIDs
	// this can be used to construct the state before the event from the
	// current state.
	StateBeforeAddsEventIDs []string
	// The state event IDs that are part of the current state, but not part
	// of the state at the event.
	StateBeforeRemovesEventIDs []string
}

// UnmarshalJSON implements json.Unmarshaller
func (ore *OutputRoomEvent) UnmarshalJSON(data []byte) error {
	// Create a struct rather than unmarshalling directly into the OutputRoomEvent
	// so that we can use json.RawMessage.
	// We use json.RawMessage so that the event JSON is sent as JSON rather than
	// being base64 encoded which is the default for []byte.
	var content struct {
		Event                      *json.RawMessage
		LatestEventIDs             []string
		AddsStateEventIDs          []string
		RemovesStateEventIDs       []string
		LastSentEventID            string
		StateBeforeAddsEventIDs    []string
		StateBeforeRemovesEventIDs []string
	}
	if err := json.Unmarshal(data, &content); err != nil {
		return err
	}
	if content.Event != nil {
		ore.Event = []byte(*content.Event)
	}
	ore.LatestEventIDs = content.LatestEventIDs
	ore.AddsStateEventIDs = content.AddsStateEventIDs
	ore.RemovesStateEventIDs = content.RemovesStateEventIDs
	ore.LastSentEventID = content.LastSentEventID
	ore.StateBeforeAddsEventIDs = content.StateBeforeAddsEventIDs
	ore.StateBeforeRemovesEventIDs = content.StateBeforeRemovesEventIDs
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
		Event                      *json.RawMessage
		LatestEventIDs             []string
		AddsStateEventIDs          []string
		RemovesStateEventIDs       []string
		LastSentEventID            string
		StateBeforeAddsEventIDs    []string
		StateBeforeRemovesEventIDs []string
	}{
		Event:                      &event,
		LatestEventIDs:             ore.LatestEventIDs,
		AddsStateEventIDs:          ore.AddsStateEventIDs,
		RemovesStateEventIDs:       ore.RemovesStateEventIDs,
		LastSentEventID:            ore.LastSentEventID,
		StateBeforeAddsEventIDs:    ore.StateBeforeAddsEventIDs,
		StateBeforeRemovesEventIDs: ore.StateBeforeRemovesEventIDs,
	}
	return json.Marshal(&content)
}
