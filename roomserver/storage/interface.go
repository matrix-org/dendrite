// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package storage

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	// Store the room state at an event in the database
	AddState(
		ctx context.Context,
		roomNID types.RoomNID,
		stateBlockNIDs []types.StateBlockNID,
		state []types.StateEntry,
	) (types.StateSnapshotNID, error)
	// Look up the state of a room at each event for a list of string event IDs.
	// Returns an error if there is an error talking to the database.
	// The length of []types.StateAtEvent is guaranteed to equal the length of eventIDs if no error is returned.
	// Returns a types.MissingEventError if the room state for the event IDs aren't in the database
	StateAtEventIDs(ctx context.Context, eventIDs []string) ([]types.StateAtEvent, error)
	// Look up the numeric IDs for a list of string event types.
	// Returns a map from string event type to numeric ID for the event type.
	EventTypeNIDs(ctx context.Context, eventTypes []string) (map[string]types.EventTypeNID, error)
	// Look up the numeric IDs for a list of string event state keys.
	// Returns a map from string state key to numeric ID for the state key.
	EventStateKeyNIDs(ctx context.Context, eventStateKeys []string) (map[string]types.EventStateKeyNID, error)
	// Look up the numeric state data IDs for each numeric state snapshot ID
	// The returned slice is sorted by numeric state snapshot ID.
	StateBlockNIDs(ctx context.Context, stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error)
	// Look up the state data for each numeric state data ID
	// The returned slice is sorted by numeric state data ID.
	StateEntries(ctx context.Context, stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error)
	// Look up the state data for the state key tuples for each numeric state block ID
	// This is used to fetch a subset of the room state at a snapshot.
	// If a block doesn't contain any of the requested tuples then it can be discarded from the result.
	// The returned slice is sorted by numeric state block ID.
	StateEntriesForTuples(
		ctx context.Context,
		stateBlockNIDs []types.StateBlockNID,
		stateKeyTuples []types.StateKeyTuple,
	) ([]types.StateEntryList, error)
	// Look up the Events for a list of numeric event IDs.
	// Returns a sorted list of events.
	Events(ctx context.Context, eventNIDs []types.EventNID) ([]types.Event, error)
	// Look up snapshot NID for an event ID string
	SnapshotNIDFromEventID(ctx context.Context, eventID string) (types.StateSnapshotNID, error)
	// Look up a room version from the room NID.
	GetRoomVersionForRoomNID(ctx context.Context, roomNID types.RoomNID) (gomatrixserverlib.RoomVersion, error)
	StoreEvent(ctx context.Context, event gomatrixserverlib.Event, txnAndSessionID *api.TransactionID, authEventNIDs []types.EventNID) (types.RoomNID, types.StateAtEvent, error)
	StateEntriesForEventIDs(ctx context.Context, eventIDs []string) ([]types.StateEntry, error)
	EventStateKeys(ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error)
	EventNIDs(ctx context.Context, eventIDs []string) (map[string]types.EventNID, error)
	SetState(ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	EventIDs(ctx context.Context, eventNIDs []types.EventNID) (map[types.EventNID]string, error)
	GetLatestEventsForUpdate(ctx context.Context, roomNID types.RoomNID) (types.RoomRecentEventsUpdater, error)
	GetTransactionEventID(ctx context.Context, transactionID string, sessionID int64, userID string) (string, error)
	RoomNID(ctx context.Context, roomID string) (types.RoomNID, error)
	// RoomNIDExcludingStubs is a special variation of RoomNID that will return 0 as if the room
	// does not exist if the room has no latest events. This can happen when we've received an
	// invite over federation for a room that we don't know anything else about yet.
	RoomNIDExcludingStubs(ctx context.Context, roomID string) (types.RoomNID, error)
	LatestEventIDs(ctx context.Context, roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, int64, error)
	GetInvitesForUser(ctx context.Context, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID) (senderUserIDs []types.EventStateKeyNID, err error)
	SetRoomAlias(ctx context.Context, alias string, roomID string, creatorUserID string) error
	GetRoomIDForAlias(ctx context.Context, alias string) (string, error)
	GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error)
	GetCreatorIDForAlias(ctx context.Context, alias string) (string, error)
	RemoveRoomAlias(ctx context.Context, alias string) error
	MembershipUpdater(ctx context.Context, roomID, targetUserID string, targetLocal bool, roomVersion gomatrixserverlib.RoomVersion) (types.MembershipUpdater, error)
	GetMembership(ctx context.Context, roomNID types.RoomNID, requestSenderUserID string) (membershipEventNID types.EventNID, stillInRoom bool, err error)
	GetMembershipEventNIDsForRoom(ctx context.Context, roomNID types.RoomNID, joinOnly bool) ([]types.EventNID, error)
	EventsFromIDs(ctx context.Context, eventIDs []string) ([]types.Event, error)
	GetRoomVersionForRoom(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error)
}
