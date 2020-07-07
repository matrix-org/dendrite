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
	// Stores a matrix room event in the database. Returns the room NID, the state snapshot and the redacted event ID if any, or an error.
	StoreEvent(
		ctx context.Context, event gomatrixserverlib.Event, txnAndSessionID *api.TransactionID, authEventNIDs []types.EventNID,
	) (types.RoomNID, types.StateAtEvent, *gomatrixserverlib.Event, string, error)
	// Look up the state entries for a list of string event IDs
	// Returns an error if the there is an error talking to the database
	// Returns a types.MissingEventError if the event IDs aren't in the database.
	StateEntriesForEventIDs(ctx context.Context, eventIDs []string) ([]types.StateEntry, error)
	// Look up the string event state keys for a list of numeric event state keys
	// Returns an error if there was a problem talking to the database.
	EventStateKeys(ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error)
	// Look up the numeric IDs for a list of events.
	// Returns an error if there was a problem talking to the database.
	EventNIDs(ctx context.Context, eventIDs []string) (map[string]types.EventNID, error)
	// Set the state at an event. FIXME TODO: "at"
	SetState(ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	// Lookup the event IDs for a batch of event numeric IDs.
	// Returns an error if the retrieval went wrong.
	EventIDs(ctx context.Context, eventNIDs []types.EventNID) (map[types.EventNID]string, error)
	// Look up the latest events in a room in preparation for an update.
	// The RoomRecentEventsUpdater must have Commit or Rollback called on it if this doesn't return an error.
	// Returns the latest events in the room and the last eventID sent to the log along with an updater.
	// If this returns an error then no further action is required.
	GetLatestEventsForUpdate(ctx context.Context, roomNID types.RoomNID) (types.RoomRecentEventsUpdater, error)
	// Look up event ID by transaction's info.
	// This is used to determine if the room event is processed/processing already.
	// Returns an empty string if no such event exists.
	GetTransactionEventID(ctx context.Context, transactionID string, sessionID int64, userID string) (string, error)
	// Look up the numeric ID for the room.
	// Returns 0 if the room doesn't exists.
	// Returns an error if there was a problem talking to the database.
	RoomNID(ctx context.Context, roomID string) (types.RoomNID, error)
	// RoomNIDExcludingStubs is a special variation of RoomNID that will return 0 as if the room
	// does not exist if the room has no latest events. This can happen when we've received an
	// invite over federation for a room that we don't know anything else about yet.
	RoomNIDExcludingStubs(ctx context.Context, roomID string) (types.RoomNID, error)
	// Look up event references for the latest events in the room and the current state snapshot.
	// Returns the latest events, the current state and the maximum depth of the latest events plus 1.
	// Returns an error if there was a problem talking to the database.
	LatestEventIDs(ctx context.Context, roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, int64, error)
	// Look up the active invites targeting a user in a room and return the
	// numeric state key IDs for the user IDs who sent them along with the event IDs for the invites.
	// Returns an error if there was a problem talking to the database.
	GetInvitesForUser(ctx context.Context, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID) (senderUserIDs []types.EventStateKeyNID, eventIDs []string, err error)
	// Save a given room alias with the room ID it refers to.
	// Returns an error if there was a problem talking to the database.
	SetRoomAlias(ctx context.Context, alias string, roomID string, creatorUserID string) error
	// Look up the room ID a given alias refers to.
	// Returns an error if there was a problem talking to the database.
	GetRoomIDForAlias(ctx context.Context, alias string) (string, error)
	// Look up all aliases referring to a given room ID.
	// Returns an error if there was a problem talking to the database.
	GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error)
	// Get the user ID of the creator of an alias.
	// Returns an error if there was a problem talking to the database.
	GetCreatorIDForAlias(ctx context.Context, alias string) (string, error)
	// Remove a given room alias.
	// Returns an error if there was a problem talking to the database.
	RemoveRoomAlias(ctx context.Context, alias string) error
	// Build a membership updater for the target user in a room.
	MembershipUpdater(ctx context.Context, roomID, targetUserID string, targetLocal bool, roomVersion gomatrixserverlib.RoomVersion) (types.MembershipUpdater, error)
	// Lookup the membership of a given user in a given room.
	// Returns the numeric ID of the latest membership event sent from this user
	// in this room, along a boolean set to true if the user is still in this room,
	// false if not.
	// Returns an error if there was a problem talking to the database.
	GetMembership(ctx context.Context, roomNID types.RoomNID, requestSenderUserID string) (membershipEventNID types.EventNID, stillInRoom bool, err error)
	// Lookup the membership event numeric IDs for all user that are or have
	// been members of a given room. Only lookup events of "join" membership if
	// joinOnly is set to true.
	// Returns an error if there was a problem talking to the database.
	GetMembershipEventNIDsForRoom(ctx context.Context, roomNID types.RoomNID, joinOnly bool, localOnly bool) ([]types.EventNID, error)
	// EventsFromIDs looks up the Events for a list of event IDs. Does not error if event was
	// not found.
	// Returns an error if the retrieval went wrong.
	EventsFromIDs(ctx context.Context, eventIDs []string) ([]types.Event, error)
	// Look up the room version for a given room.
	GetRoomVersionForRoom(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error)
	// Publish or unpublish a room from the room directory.
	PublishRoom(ctx context.Context, roomID string, publish bool) error
	// Returns a list of room IDs for rooms which are published.
	GetPublishedRooms(ctx context.Context) ([]string, error)
}
