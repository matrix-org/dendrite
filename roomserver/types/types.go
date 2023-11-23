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
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"golang.org/x/crypto/blake2b"
)

// EventTypeNID is a numeric ID for an event type.
type EventTypeNID int64

// EventStateKeyNID is a numeric ID for an event state_key.
type EventStateKeyNID int64

// EventNID is a numeric ID for an event.
type EventNID int64

// RoomNID is a numeric ID for a room.
type RoomNID int64

type EventMetadata struct {
	EventNID EventNID
	RoomNID  RoomNID
}

type UserRoomKeyPair struct {
	RoomNID          RoomNID
	EventStateKeyNID EventStateKeyNID
}

// StateSnapshotNID is a numeric ID for the state at an event.
type StateSnapshotNID int64

// StateBlockNID is a numeric ID for a block of state data.
// These blocks of state data are combined to form the actual state.
type StateBlockNID int64

// EventNIDs is used to sort and dedupe event NIDs.
type EventNIDs []EventNID

func (a EventNIDs) Len() int           { return len(a) }
func (a EventNIDs) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a EventNIDs) Less(i, j int) bool { return a[i] < a[j] }

func (a EventNIDs) Hash() []byte {
	j, err := json.Marshal(a)
	if err != nil {
		return nil
	}
	h := blake2b.Sum256(j)
	return h[:]
}

// StateBlockNIDs is used to sort and dedupe state block NIDs.
type StateBlockNIDs []StateBlockNID

func (a StateBlockNIDs) Len() int           { return len(a) }
func (a StateBlockNIDs) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a StateBlockNIDs) Less(i, j int) bool { return a[i] < a[j] }

func (a StateBlockNIDs) Hash() []byte {
	j, err := json.Marshal(a)
	if err != nil {
		return nil
	}
	h := blake2b.Sum256(j)
	return h[:]
}

// A StateKeyTuple is a pair of a numeric event type and a numeric state key.
// It is used to lookup state entries.
type StateKeyTuple struct {
	// The numeric ID for the event type.
	EventTypeNID EventTypeNID
	// The numeric ID for the state key.
	EventStateKeyNID EventStateKeyNID
}

func (a StateKeyTuple) IsCreate() bool {
	return a.EventTypeNID == MRoomCreateNID && a.EventStateKeyNID == EmptyStateKeyNID
}

// LessThan returns true if this state key is less than the other state key.
// The ordering is arbitrary and is used to implement binary search and to efficiently deduplicate entries.
func (a StateKeyTuple) LessThan(b StateKeyTuple) bool {
	if a.EventTypeNID != b.EventTypeNID {
		return a.EventTypeNID < b.EventTypeNID
	}
	return a.EventStateKeyNID < b.EventStateKeyNID
}

type StateKeyTupleSorter []StateKeyTuple

func (s StateKeyTupleSorter) Len() int           { return len(s) }
func (s StateKeyTupleSorter) Less(i, j int) bool { return s[i].LessThan(s[j]) }
func (s StateKeyTupleSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Check whether a tuple is in the list. Assumes that the list is sorted.
func (s StateKeyTupleSorter) contains(value StateKeyTuple) bool {
	i := sort.Search(len(s), func(i int) bool { return !s[i].LessThan(value) })
	return i < len(s) && s[i] == value
}

// List the unique eventTypeNIDs and eventStateKeyNIDs.
// Assumes that the list is sorted.
func (s StateKeyTupleSorter) TypesAndStateKeysAsArrays() (eventTypeNIDs []int64, eventStateKeyNIDs []int64) {
	eventTypeNIDs = make([]int64, len(s))
	eventStateKeyNIDs = make([]int64, len(s))
	for i := range s {
		eventTypeNIDs[i] = int64(s[i].EventTypeNID)
		eventStateKeyNIDs[i] = int64(s[i].EventStateKeyNID)
	}
	eventTypeNIDs = eventTypeNIDs[:util.SortAndUnique(int64Sorter(eventTypeNIDs))]
	eventStateKeyNIDs = eventStateKeyNIDs[:util.SortAndUnique(int64Sorter(eventStateKeyNIDs))]
	return
}

type int64Sorter []int64

func (s int64Sorter) Len() int           { return len(s) }
func (s int64Sorter) Less(i, j int) bool { return s[i] < s[j] }
func (s int64Sorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// A StateEntry is an entry in the room state of a matrix room.
type StateEntry struct {
	StateKeyTuple
	// The numeric ID for the event.
	EventNID EventNID
}

type StateEntries []StateEntry

func (a StateEntries) Len() int           { return len(a) }
func (a StateEntries) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a StateEntries) Less(i, j int) bool { return a[i].EventNID < a[j].EventNID }

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
	EventID string
}

type StateAtEventAndReferences []StateAtEventAndReference

func (s StateAtEventAndReferences) Less(a, b int) bool {
	return strings.Compare(s[a].EventID, s[b].EventID) < 0
}

func (s StateAtEventAndReferences) Len() int {
	return len(s)
}

func (s StateAtEventAndReferences) Swap(a, b int) {
	s[a], s[b] = s[b], s[a]
}

func (s StateAtEventAndReferences) EventIDs() string {
	strs := make([]string, 0, len(s))
	for _, r := range s {
		strs = append(strs, r.EventID)
	}
	return "[" + strings.Join(strs, " ") + "]"
}

// An Event is a gomatrixserverlib.Event with the numeric event ID attached.
// It is when performing bulk event lookup in the database.
type Event struct {
	EventNID EventNID
	gomatrixserverlib.PDU
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

// A MissingStateError is an error that happened because the roomserver was
// missing requested state snapshots from its databases.
type MissingStateError string

func (e MissingStateError) Error() string { return string(e) }

// A RejectedError is returned when an event is stored as rejected. The error
// contains the reason why.
type RejectedError string

func (e RejectedError) Error() string { return string(e) }

// RoomInfo contains metadata about a room
type RoomInfo struct {
	mu               sync.RWMutex
	RoomNID          RoomNID
	RoomVersion      gomatrixserverlib.RoomVersion
	stateSnapshotNID StateSnapshotNID
	isStub           bool
}

func (r *RoomInfo) StateSnapshotNID() StateSnapshotNID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stateSnapshotNID
}

func (r *RoomInfo) IsStub() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isStub
}

func (r *RoomInfo) SetStateSnapshotNID(nid StateSnapshotNID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stateSnapshotNID = nid
}

func (r *RoomInfo) SetIsStub(isStub bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.isStub = isStub
}

func (r *RoomInfo) CopyFrom(r2 *RoomInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r2.mu.RLock()
	defer r2.mu.RUnlock()

	r.RoomNID = r2.RoomNID
	r.RoomVersion = r2.RoomVersion
	r.stateSnapshotNID = r2.stateSnapshotNID
	r.isStub = r2.isStub
}

var ErrorInvalidRoomInfo = fmt.Errorf("room info is invalid")

// Struct to represent a device or a server name.
//
// May be used to designate a caller for functions that can be called
// by a client (device) or by a server (server name).
//
// Exactly 1 of Device() and ServerName() will return a non-nil result.
type DeviceOrServerName struct {
	device     *userapi.Device
	serverName *spec.ServerName
}

func NewDeviceNotServerName(device userapi.Device) DeviceOrServerName {
	return DeviceOrServerName{
		device:     &device,
		serverName: nil,
	}
}

func NewServerNameNotDevice(serverName spec.ServerName) DeviceOrServerName {
	return DeviceOrServerName{
		device:     nil,
		serverName: &serverName,
	}
}

func (s *DeviceOrServerName) Device() *userapi.Device {
	return s.device
}

func (s *DeviceOrServerName) ServerName() *spec.ServerName {
	return s.serverName
}
