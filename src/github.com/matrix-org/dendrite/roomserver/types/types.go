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

// StateAtEvent is the state before and after a matrix event.
type StateAtEvent struct {
	// The state before the event.
	BeforeStateSnapshotNID StateSnapshotNID
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

// A Transaction is something that can be committed or rolledback.
type Transaction interface {
	// Commit the transaction
	Commit() error
	// Rollback the transaction.
	Rollback() error
}

// A RoomRecentEventsUpdater is used to update the recent events in a room.
// (On postgresql this wraps a database transaction that holds a "FOR UPDATE"
//  lock on the row in the rooms table holding the latest events for the room.)
type RoomRecentEventsUpdater interface {
	// The latest event IDs and state in the room.
	LatestEvents() []StateAtEventAndReference
	// The event ID of the latest event written to the output log in the room.
	LastEventIDSent() string
	// The current state of the room.
	CurrentStateSnapshotNID() StateSnapshotNID
	// Store the previous events referenced by an event.
	// This adds the event NID to an entry in the database for each of the previous events.
	// If there isn't an entry for one of previous events then an entry is created.
	// If the entry already lists the event NID as a referrer then the entry unmodified.
	// (i.e. the operation is idempotent)
	StorePreviousEvents(eventNID EventNID, previousEventReferences []gomatrixserverlib.EventReference) error
	// Check whether the eventReference is already referenced by another matrix event.
	IsReferenced(eventReference gomatrixserverlib.EventReference) (bool, error)
	// Set the list of latest events for the room.
	// This replaces the current list stored in the database with the given list
	SetLatestEvents(
		roomNID RoomNID, latest []StateAtEventAndReference, lastEventNIDSent EventNID,
		currentStateSnapshotNID StateSnapshotNID,
	) error
	// Check if the event has already be written to the output logs.
	HasEventBeenSent(eventNID EventNID) (bool, error)
	// Mark the event as having been sent to the output logs.
	MarkEventAsSent(eventNID EventNID) error
	// Build a membership updater for the target user in this room.
	// It will share the same transaction as this updater.
	MembershipUpdater(targetUserNID EventStateKeyNID) (MembershipUpdater, error)
	// Implements Transaction so it can be committed or rolledback
	Transaction
}

// A MembershipUpdater is used to update the membership of a user in a room.
// (On postgresql this wraps a database transaction that holds a "FOR UPDATE"
//  lock on the row in the membership table for this user in the room)
// The caller should call one of SetToInvite, SetToJoin or SetToLeave once to
// make the update, or none of them if no update is required.
type MembershipUpdater interface {
	// True if the target user is invited to the room.
	IsInvite() bool
	// True if the target user is joined to the room.
	IsJoin() bool
	// True if the target user is not invited or joined to the room.
	IsLeave() bool
	// Set the state to invite.
	// Returns whether this invite needs to be sent
	SetToInvite(event gomatrixserverlib.Event) (needsSending bool, err error)
	// Set the state to join.
	// Returns a list of invite event IDs that this state change retired.
	SetToJoin(senderUserID string) (inviteEventIDs []string, err error)
	// Set the state to leave.
	// Returns a list of invite event IDs that this state change retired.
	SetToLeave(senderUserID string) (inviteEventIDs []string, err error)
	// Implements Transaction so it can be committed or rolledback.
	Transaction
}

// A MissingEventError is an error that happened because the roomserver was
// missing requested events from its database.
type MissingEventError string

func (e MissingEventError) Error() string { return string(e) }
