// Package types provides the types that are used internally within the roomserver.
package types

import (
	"github.com/matrix-org/gomatrixserverlib"
)

// A PartitionOffset is the offset into a partition of the input log.
type PartitionOffset struct {
	// The ID of the partition.
	Partition int32
	// The offset into the partition.
	Offset int64
}

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

// StateAtEvent is the state before and after a matrix event.
type StateAtEvent struct {
	// The state before the event.
	BeforeStateSnapshotNID StateSnapshotNID
	// The state entry for the event itself, allows us to calculate the state after the event.
	StateEntry
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
