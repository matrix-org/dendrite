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

// Package types provides the types that are used internally within the roomserver.
package types

import (
	"sort"

	"github.com/matrix-org/gomatrixserverlib"
)

// EventTypeNID is a numeric ID for an event type.
type EventTypeNID int64

// EventStateKeyNID is a numeric ID for an event state_key.
type EventStateKeyNID int64

// EventNID is a numeric ID for an event.
type EventNID int64

// RoomNID is a numeric ID for a room.
type RoomNID int64

// StateSnapshotNID is a numeric ID for the state at an event.
type StateSnapshotNID int64

// StateBlockNID is a numeric ID for a block of state data.
// These blocks of state data are combined to form the actual state.
type StateBlockNID int64

// A StateKeyTuple is a pair of a numeric event type and a numeric state key.
// It is used to lookup state entries.
type StateKeyTuple struct {
	// The numeric ID for the event type.
	EventTypeNID EventTypeNID
	// The numeric ID for the state key.
	EventStateKeyNID EventStateKeyNID
}

// LessThan returns true if this state key is less than the other state key.
// The ordering is arbitrary and is used to implement binary search and to efficiently deduplicate entries.
func (a StateKeyTuple) LessThan(b StateKeyTuple) bool {
	if a.EventTypeNID != b.EventTypeNID {
		return a.EventTypeNID < b.EventTypeNID
	}
	return a.EventStateKeyNID < b.EventStateKeyNID
}

// A StateEntry is an entry in the room state of a matrix room.
type StateEntry struct {
	StateKeyTuple
	// The numeric ID for the event.
	EventNID EventNID
}

// LessThan returns true if this state entry is less than the other state entry.
// The ordering is arbitrary and is used to implement binary search and to efficiently deduplicate entries.
func (a StateEntry) LessThan(b StateEntry) bool {
	if a.StateKeyTuple != b.StateKeyTuple {
		return a.StateKeyTuple.LessThan(b.StateKeyTuple)
	}
	return a.EventNID < b.EventNID
}

// Deduplicate takes a set of state entries and ensures that there are no
// duplicate (event type, state key) tuples. If there are then we dedupe
// them, making sure that the latest/highest NIDs are always chosen.
func DeduplicateStateEntries(a []StateEntry) []StateEntry {
	if len(a) < 2 {
		return a
	}
	sort.SliceStable(a, func(i, j int) bool {
		return a[i].LessThan(a[j])
	})
	for i := 0; i < len(a)-1; i++ {
		if a[i].StateKeyTuple == a[i+1].StateKeyTuple {
			a = append(a[:i], a[i+1:]...)
			i--
		}
	}
	return a
}

// StateAtEvent is the state before and after a matrix event.
type StateAtEvent struct {
	// Should this state overwrite the latest events and memberships of the room?
	// This might be necessary when rejoining a federated room after a period of
	// absence, as our state and latest events will be out of date.
	Overwrite bool
	// The state before the event.
	BeforeStateSnapshotNID StateSnapshotNID
	// True if this StateEntry is rejected. State resolution should then treat this
	// StateEntry as being a message event (not a state event).
	IsRejected bool
	// The state entry for the event itself, allows us to calculate the state after the event.
	StateEntry
}

// IsStateEvent returns whether the event the state is at is a state event.
func (s StateAtEvent) IsStateEvent() bool {
	return s.EventStateKeyNID != 0
}

// StateAtEventAndReference is StateAtEvent and gomatrixserverlib.EventReference glued together.
// It is used when looking up the latest events in a room in the database.
// The gomatrixserverlib.EventReference is used to check whether a new event references the event.
// The StateAtEvent is used to construct the current state of the room from the latest events.
type StateAtEventAndReference struct {
	StateAtEvent
	gomatrixserverlib.EventReference
}

// An Event is a gomatrixserverlib.Event with the numeric event ID attached.
// It is when performing bulk event lookup in the database.
type Event struct {
	EventNID EventNID
	gomatrixserverlib.Event
}

const (
	// MRoomCreateNID is the numeric ID for the "m.room.create" event type.
	MRoomCreateNID = 1
	// MRoomPowerLevelsNID is the numeric ID for the "m.room.power_levels" event type.
	MRoomPowerLevelsNID = 2
	// MRoomJoinRulesNID is the numeric ID for the "m.room.join_rules" event type.
	MRoomJoinRulesNID = 3
	// MRoomThirdPartyInviteNID is the numeric ID for the "m.room.third_party_invite" event type.
	MRoomThirdPartyInviteNID = 4
	// MRoomMemberNID is the numeric ID for the "m.room.member" event type.
	MRoomMemberNID = 5
	// MRoomRedactionNID is the numeric ID for the "m.room.redaction" event type.
	MRoomRedactionNID = 6
	// MRoomHistoryVisibilityNID is the numeric ID for the "m.room.history_visibility" event type.
	MRoomHistoryVisibilityNID = 7
)

const (
	// EmptyStateKeyNID is the numeric ID for the empty state key.
	EmptyStateKeyNID = 1
)

// StateBlockNIDList is used to return the result of bulk StateBlockNID lookups from the database.
type StateBlockNIDList struct {
	StateSnapshotNID StateSnapshotNID
	StateBlockNIDs   []StateBlockNID
}

// StateEntryList is used to return the result of bulk state entry lookups from the database.
type StateEntryList struct {
	StateBlockNID StateBlockNID
	StateEntries  []StateEntry
}

// A MissingEventError is an error that happened because the roomserver was
// missing requested events from its database.
type MissingEventError string

func (e MissingEventError) Error() string { return string(e) }

// RoomInfo contains metadata about a room
type RoomInfo struct {
	RoomNID          RoomNID
	RoomVersion      gomatrixserverlib.RoomVersion
	StateSnapshotNID StateSnapshotNID
	IsStub           bool
}
