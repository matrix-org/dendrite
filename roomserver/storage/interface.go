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
	"crypto/ed25519"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type Database interface {
	UserRoomKeys
	ReportedEvents
	// Do we support processing input events for more than one room at a time?
	SupportsConcurrentRoomInputs() bool
	AssignRoomNID(ctx context.Context, roomID spec.RoomID, roomVersion gomatrixserverlib.RoomVersion) (roomNID types.RoomNID, err error)
	// RoomInfo returns room information for the given room ID, or nil if there is no room.
	RoomInfo(ctx context.Context, roomID string) (*types.RoomInfo, error)
	RoomInfoByNID(ctx context.Context, roomNID types.RoomNID) (*types.RoomInfo, error)
	// Store the room state at an event in the database
	AddState(
		ctx context.Context,
		roomNID types.RoomNID,
		stateBlockNIDs []types.StateBlockNID,
		state []types.StateEntry,
	) (types.StateSnapshotNID, error)

	MissingAuthPrevEvents(
		ctx context.Context, e gomatrixserverlib.PDU,
	) (missingAuth, missingPrev []string, err error)

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
	Events(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, eventNIDs []types.EventNID) ([]types.Event, error)
	// Look up snapshot NID for an event ID string
	SnapshotNIDFromEventID(ctx context.Context, eventID string) (types.StateSnapshotNID, error)
	BulkSelectSnapshotsFromEventIDs(ctx context.Context, eventIDs []string) (map[types.StateSnapshotNID][]string, error)
	// Stores a matrix room event in the database. Returns the room NID, the state snapshot or an error.
	StoreEvent(ctx context.Context, event gomatrixserverlib.PDU, roomInfo *types.RoomInfo, eventTypeNID types.EventTypeNID, eventStateKeyNID types.EventStateKeyNID, authEventNIDs []types.EventNID, isRejected bool) (types.EventNID, types.StateAtEvent, error)
	// Look up the state entries for a list of string event IDs
	// Returns an error if the there is an error talking to the database
	// Returns a types.MissingEventError if the event IDs aren't in the database.
	StateEntriesForEventIDs(ctx context.Context, eventIDs []string, excludeRejected bool) ([]types.StateEntry, error)
	// Look up the string event state keys for a list of numeric event state keys
	// Returns an error if there was a problem talking to the database.
	EventStateKeys(ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error)
	// Look up the numeric IDs for a list of events.
	// Returns an error if there was a problem talking to the database.
	EventNIDs(ctx context.Context, eventIDs []string) (map[string]types.EventMetadata, error)
	// Set the state at an event. FIXME TODO: "at"
	SetState(ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	// Lookup the event IDs for a batch of event numeric IDs.
	// Returns an error if the retrieval went wrong.
	EventIDs(ctx context.Context, eventNIDs []types.EventNID) (map[types.EventNID]string, error)
	// Opens and returns a room updater, which locks the room and opens a transaction.
	// The GetRoomUpdater must have Commit or Rollback called on it if this doesn't return an error.
	// If this returns an error then no further action is required.
	// IsEventRejected returns true if the event is known and rejected.
	IsEventRejected(ctx context.Context, roomNID types.RoomNID, eventID string) (rejected bool, err error)
	GetRoomUpdater(ctx context.Context, roomInfo *types.RoomInfo) (*shared.RoomUpdater, error)
	// Look up event references for the latest events in the room and the current state snapshot.
	// Returns the latest events, the current state and the maximum depth of the latest events plus 1.
	// Returns an error if there was a problem talking to the database.
	LatestEventIDs(ctx context.Context, roomNID types.RoomNID) ([]string, types.StateSnapshotNID, int64, error)
	// Look up the active invites targeting a user in a room and return the
	// numeric state key IDs for the user IDs who sent them along with the event IDs for the invites.
	// Returns an error if there was a problem talking to the database.
	GetInvitesForUser(ctx context.Context, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID) (senderUserIDs []types.EventStateKeyNID, eventIDs []string, inviteEventJSON []byte, err error)
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
	MembershipUpdater(ctx context.Context, roomID, targetUserID string, targetLocal bool, roomVersion gomatrixserverlib.RoomVersion) (*shared.MembershipUpdater, error)
	// Lookup the membership of a given user in a given room.
	// Returns the numeric ID of the latest membership event sent from this user
	// in this room, along a boolean set to true if the user is still in this room,
	// false if not.
	// Returns an error if there was a problem talking to the database.
	GetMembership(ctx context.Context, roomNID types.RoomNID, requestSenderID spec.SenderID) (membershipEventNID types.EventNID, stillInRoom, isRoomForgotten bool, err error)
	// Lookup the membership event numeric IDs for all user that are or have
	// been members of a given room. Only lookup events of "join" membership if
	// joinOnly is set to true.
	// Returns an error if there was a problem talking to the database.
	GetMembershipEventNIDsForRoom(ctx context.Context, roomNID types.RoomNID, joinOnly bool, localOnly bool) ([]types.EventNID, error)
	// EventsFromIDs looks up the Events for a list of event IDs. Does not error if event was
	// not found.
	// Returns an error if the retrieval went wrong.
	EventsFromIDs(ctx context.Context, roomInfo *types.RoomInfo, eventIDs []string) ([]types.Event, error)
	// PerformPublish publishes or unpublishes a room from the room directory. Returns a database error, if any.
	PublishRoom(ctx context.Context, roomID, appserviceID, networkID string, publish bool) error
	// Returns a list of room IDs for rooms which are published.
	GetPublishedRooms(ctx context.Context, networkID string, includeAllNetworks bool) ([]string, error)
	// Returns whether a given room is published or not.
	GetPublishedRoom(ctx context.Context, roomID string) (bool, error)

	// TODO: factor out - from currentstateserver

	// GetStateEvent returns the state event of a given type for a given room with a given state key
	// If no event could be found, returns nil
	// If there was an issue during the retrieval, returns an error
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*types.HeaderedEvent, error)
	GetStateEventsWithEventType(ctx context.Context, roomID, evType string) ([]*types.HeaderedEvent, error)
	// GetRoomsByMembership returns a list of room IDs matching the provided membership and user ID (as state_key).
	GetRoomsByMembership(ctx context.Context, userID spec.UserID, membership string) ([]string, error)
	// GetBulkStateContent returns all state events which match a given room ID and a given state key tuple. Both must be satisfied for a match.
	// If a tuple has the StateKey of '*' and allowWildcards=true then all state events with the EventType should be returned.
	GetBulkStateContent(ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool) ([]tables.StrippedEvent, error)
	// JoinedUsersSetInRooms returns how many times each of the given users appears across the given rooms.
	JoinedUsersSetInRooms(ctx context.Context, roomIDs, userIDs []string, localOnly bool) (map[string]int, error)
	// GetLocalServerInRoom returns true if we think we're in a given room or false otherwise.
	GetLocalServerInRoom(ctx context.Context, roomNID types.RoomNID) (bool, error)
	// GetServerInRoom returns true if we think a server is in a given room or false otherwise.
	GetServerInRoom(ctx context.Context, roomNID types.RoomNID, serverName spec.ServerName) (bool, error)
	// GetKnownUsers searches all users that userID knows about.
	GetKnownUsers(ctx context.Context, userID, searchString string, limit int) ([]string, error)
	// ForgetRoom sets a flag in the membership table, that the user wishes to forget a specific room
	ForgetRoom(ctx context.Context, userID, roomID string, forget bool) error

	GetHistoryVisibilityState(ctx context.Context, roomInfo *types.RoomInfo, eventID string, domain string) ([]gomatrixserverlib.PDU, error)
	GetLeftUsers(ctx context.Context, userIDs []string) ([]string, error)
	PurgeRoom(ctx context.Context, roomID string) error
	UpgradeRoom(ctx context.Context, oldRoomID, newRoomID, eventSender string) error

	// GetMembershipForHistoryVisibility queries the membership events for the given eventIDs.
	// Returns a map from (input) eventID -> membership event. If no membership event is found, returns an empty event, resulting in
	// a membership of "leave" when calculating history visibility.
	GetMembershipForHistoryVisibility(
		ctx context.Context, userNID types.EventStateKeyNID, info *types.RoomInfo, eventIDs ...string,
	) (map[string]*types.HeaderedEvent, error)
	GetOrCreateRoomInfo(ctx context.Context, event gomatrixserverlib.PDU) (*types.RoomInfo, error)
	GetRoomVersion(ctx context.Context, roomID string) (gomatrixserverlib.RoomVersion, error)
	GetOrCreateEventTypeNID(ctx context.Context, eventType string) (eventTypeNID types.EventTypeNID, err error)
	GetOrCreateEventStateKeyNID(ctx context.Context, eventStateKey *string) (types.EventStateKeyNID, error)
	MaybeRedactEvent(
		ctx context.Context, roomInfo *types.RoomInfo, eventNID types.EventNID, event gomatrixserverlib.PDU, plResolver state.PowerLevelResolver, querier api.QuerySenderIDAPI,
	) (gomatrixserverlib.PDU, gomatrixserverlib.PDU, error)

	// RoomsWithACLs returns all room IDs for rooms with ACLs
	RoomsWithACLs(ctx context.Context) ([]string, error)
	QueryAdminEventReports(ctx context.Context, from uint64, limit uint64, backwards bool, userID string, roomID string) ([]api.QueryAdminEventReportsResponse, int64, error)
	QueryAdminEventReport(ctx context.Context, reportID uint64) (api.QueryAdminEventReportResponse, error)
	AdminDeleteEventReport(ctx context.Context, reportID uint64) error
}

type UserRoomKeys interface {
	// InsertUserRoomPrivatePublicKey inserts the given private key as well as the public key for it. This should be used
	// when creating keys locally.
	InsertUserRoomPrivatePublicKey(ctx context.Context, userID spec.UserID, roomID spec.RoomID, key ed25519.PrivateKey) (result ed25519.PrivateKey, err error)
	// InsertUserRoomPublicKey inserts the given public key, this should be used for users NOT local to this server
	InsertUserRoomPublicKey(ctx context.Context, userID spec.UserID, roomID spec.RoomID, key ed25519.PublicKey) (result ed25519.PublicKey, err error)
	// SelectUserRoomPrivateKey selects the private key for the given user and room combination
	SelectUserRoomPrivateKey(ctx context.Context, userID spec.UserID, roomID spec.RoomID) (key ed25519.PrivateKey, err error)
	// SelectUserRoomPublicKey selects the public key for the given user and room combination
	SelectUserRoomPublicKey(ctx context.Context, userID spec.UserID, roomID spec.RoomID) (key ed25519.PublicKey, err error)
	// SelectUserIDsForPublicKeys selects all userIDs for the requested senderKeys. Returns a map from roomID -> map from publicKey to userID.
	// If a senderKey can't be found, it is omitted in the result.
	// TODO: Why is the result map indexed by string not public key?
	// TODO: Shouldn't the input & result map be changed to be indexed by string instead of the RoomID struct?
	SelectUserIDsForPublicKeys(ctx context.Context, publicKeys map[spec.RoomID][]ed25519.PublicKey) (map[spec.RoomID]map[string]string, error)
}

type RoomDatabase interface {
	EventDatabase
	UserRoomKeys
	AssignRoomNID(ctx context.Context, roomID spec.RoomID, roomVersion gomatrixserverlib.RoomVersion) (roomNID types.RoomNID, err error)
	// RoomInfo returns room information for the given room ID, or nil if there is no room.
	RoomInfo(ctx context.Context, roomID string) (*types.RoomInfo, error)
	RoomInfoByNID(ctx context.Context, roomNID types.RoomNID) (*types.RoomInfo, error)
	// IsEventRejected returns true if the event is known and rejected.
	IsEventRejected(ctx context.Context, roomNID types.RoomNID, eventID string) (rejected bool, err error)
	MissingAuthPrevEvents(ctx context.Context, e gomatrixserverlib.PDU) (missingAuth, missingPrev []string, err error)
	UpgradeRoom(ctx context.Context, oldRoomID, newRoomID, eventSender string) error
	GetRoomUpdater(ctx context.Context, roomInfo *types.RoomInfo) (*shared.RoomUpdater, error)
	GetMembershipEventNIDsForRoom(ctx context.Context, roomNID types.RoomNID, joinOnly bool, localOnly bool) ([]types.EventNID, error)
	StateBlockNIDs(ctx context.Context, stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error)
	StateEntries(ctx context.Context, stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error)
	BulkSelectSnapshotsFromEventIDs(ctx context.Context, eventIDs []string) (map[types.StateSnapshotNID][]string, error)
	StateEntriesForTuples(ctx context.Context, stateBlockNIDs []types.StateBlockNID, stateKeyTuples []types.StateKeyTuple) ([]types.StateEntryList, error)
	AddState(ctx context.Context, roomNID types.RoomNID, stateBlockNIDs []types.StateBlockNID, state []types.StateEntry) (types.StateSnapshotNID, error)
	LatestEventIDs(ctx context.Context, roomNID types.RoomNID) ([]string, types.StateSnapshotNID, int64, error)
	GetOrCreateRoomInfo(ctx context.Context, event gomatrixserverlib.PDU) (*types.RoomInfo, error)
	GetOrCreateEventTypeNID(ctx context.Context, eventType string) (eventTypeNID types.EventTypeNID, err error)
	GetOrCreateEventStateKeyNID(ctx context.Context, eventStateKey *string) (types.EventStateKeyNID, error)
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*types.HeaderedEvent, error)
}

type EventDatabase interface {
	EventTypeNIDs(ctx context.Context, eventTypes []string) (map[string]types.EventTypeNID, error)
	EventStateKeys(ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error)
	EventStateKeyNIDs(ctx context.Context, eventStateKeys []string) (map[string]types.EventStateKeyNID, error)
	StateEntriesForEventIDs(ctx context.Context, eventIDs []string, excludeRejected bool) ([]types.StateEntry, error)
	EventNIDs(ctx context.Context, eventIDs []string) (map[string]types.EventMetadata, error)
	SetState(ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	StateAtEventIDs(ctx context.Context, eventIDs []string) ([]types.StateAtEvent, error)
	SnapshotNIDFromEventID(ctx context.Context, eventID string) (types.StateSnapshotNID, error)
	EventIDs(ctx context.Context, eventNIDs []types.EventNID) (map[types.EventNID]string, error)
	EventsFromIDs(ctx context.Context, roomInfo *types.RoomInfo, eventIDs []string) ([]types.Event, error)
	Events(ctx context.Context, roomVersion gomatrixserverlib.RoomVersion, eventNIDs []types.EventNID) ([]types.Event, error)
	// MaybeRedactEvent returns the redaction event and the redacted event if this call resulted in a redaction, else an error
	// (nil if there was nothing to do)
	MaybeRedactEvent(
		ctx context.Context, roomInfo *types.RoomInfo, eventNID types.EventNID, event gomatrixserverlib.PDU, plResolver state.PowerLevelResolver, querier api.QuerySenderIDAPI,
	) (gomatrixserverlib.PDU, gomatrixserverlib.PDU, error)
	StoreEvent(ctx context.Context, event gomatrixserverlib.PDU, roomInfo *types.RoomInfo, eventTypeNID types.EventTypeNID, eventStateKeyNID types.EventStateKeyNID, authEventNIDs []types.EventNID, isRejected bool) (types.EventNID, types.StateAtEvent, error)
}

type ReportedEvents interface {
	InsertReportedEvent(
		ctx context.Context,
		roomID, eventID, reportingUserID, reason string,
		score int64,
	) (int64, error)
}
