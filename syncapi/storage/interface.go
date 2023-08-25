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

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/syncapi/storage/shared"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type DatabaseTransaction interface {
	sqlutil.Transaction
	SharedUsers

	MaxStreamPositionForPDUs(ctx context.Context) (types.StreamPosition, error)
	MaxStreamPositionForReceipts(ctx context.Context) (types.StreamPosition, error)
	MaxStreamPositionForInvites(ctx context.Context) (types.StreamPosition, error)
	MaxStreamPositionForAccountData(ctx context.Context) (types.StreamPosition, error)
	MaxStreamPositionForSendToDeviceMessages(ctx context.Context) (types.StreamPosition, error)
	MaxStreamPositionForNotificationData(ctx context.Context) (types.StreamPosition, error)
	MaxStreamPositionForPresence(ctx context.Context) (types.StreamPosition, error)
	MaxStreamPositionForRelations(ctx context.Context) (types.StreamPosition, error)

	CurrentState(ctx context.Context, roomID string, stateFilterPart *synctypes.StateFilter, excludeEventIDs []string) ([]*rstypes.HeaderedEvent, error)
	GetStateDeltasForFullStateSync(ctx context.Context, device *userapi.Device, r types.Range, userID string, stateFilter *synctypes.StateFilter, rsAPI api.SyncRoomserverAPI) ([]types.StateDelta, []string, error)
	GetStateDeltas(ctx context.Context, device *userapi.Device, r types.Range, userID string, stateFilter *synctypes.StateFilter, rsAPI api.SyncRoomserverAPI) ([]types.StateDelta, []string, error)
	RoomIDsWithMembership(ctx context.Context, userID string, membership string) ([]string, error)
	MembershipCount(ctx context.Context, roomID, membership string, pos types.StreamPosition) (int, error)
	GetRoomSummary(ctx context.Context, roomID, userID string) (summary *types.Summary, err error)
	RecentEvents(ctx context.Context, roomIDs []string, r types.Range, eventFilter *synctypes.RoomEventFilter, chronologicalOrder bool, onlySyncEvents bool) (map[string]types.RecentEvents, error)
	GetBackwardTopologyPos(ctx context.Context, events []*rstypes.HeaderedEvent) (types.TopologyToken, error)
	PositionInTopology(ctx context.Context, eventID string) (pos types.StreamPosition, spos types.StreamPosition, err error)
	InviteEventsInRange(ctx context.Context, targetUserID string, r types.Range) (map[string]*rstypes.HeaderedEvent, map[string]*rstypes.HeaderedEvent, types.StreamPosition, error)
	PeeksInRange(ctx context.Context, userID, deviceID string, r types.Range) (peeks []types.Peek, err error)
	RoomReceiptsAfter(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) (types.StreamPosition, []types.OutputReceiptEvent, error)
	// AllJoinedUsersInRooms returns a map of room ID to a list of all joined user IDs.
	AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error)
	// AllJoinedUsersInRoom returns a map of room ID to a list of all joined user IDs for a given room.
	AllJoinedUsersInRoom(ctx context.Context, roomIDs []string) (map[string][]string, error)
	// AllPeekingDevicesInRooms returns a map of room ID to a list of all peeking devices.
	AllPeekingDevicesInRooms(ctx context.Context) (map[string][]types.PeekingDevice, error)
	// Events lookups a list of event by their event ID.
	// Returns a list of events matching the requested IDs found in the database.
	// If an event is not found in the database then it will be omitted from the list.
	// Returns an error if there was a problem talking with the database.
	// Does not include any transaction IDs in the returned events.
	Events(ctx context.Context, eventIDs []string) ([]*rstypes.HeaderedEvent, error)
	// GetStateEvent returns the Matrix state event of a given type for a given room with a given state key
	// If no event could be found, returns nil
	// If there was an issue during the retrieval, returns an error
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*rstypes.HeaderedEvent, error)
	// GetStateEventsForRoom fetches the state events for a given room.
	// Returns an empty slice if no state events could be found for this room.
	// Returns an error if there was an issue with the retrieval.
	GetStateEventsForRoom(ctx context.Context, roomID string, stateFilterPart *synctypes.StateFilter) (stateEvents []*rstypes.HeaderedEvent, err error)
	// GetAccountDataInRange returns all account data for a given user inserted or
	// updated between two given positions
	// Returns a map following the format data[roomID] = []dataTypes
	// If no data is retrieved, returns an empty map
	// If there was an issue with the retrieval, returns an error
	GetAccountDataInRange(ctx context.Context, userID string, r types.Range, accountDataFilterPart *synctypes.EventFilter) (map[string][]string, types.StreamPosition, error)
	// GetEventsInTopologicalRange retrieves all of the events on a given ordering using the given extremities and limit.
	// If backwardsOrdering is true, the most recent event must be first, else last.
	// Returns the filtered StreamEvents on success. Returns **unfiltered** StreamEvents and ErrNoEventsForFilter if
	// the provided filter removed all events, this can be used to still calculate the start/end position. (e.g for `/messages`)
	GetEventsInTopologicalRange(ctx context.Context, from, to *types.TopologyToken, roomID string, filter *synctypes.RoomEventFilter, backwardOrdering bool) (events []types.StreamEvent, start, end types.TopologyToken, err error)
	// EventPositionInTopology returns the depth and stream position of the given event.
	EventPositionInTopology(ctx context.Context, eventID string) (types.TopologyToken, error)
	// BackwardExtremitiesForRoom returns a map of backwards extremity event ID to a list of its prev_events.
	BackwardExtremitiesForRoom(ctx context.Context, roomID string) (backwardExtremities map[string][]string, err error)
	// StreamEventsToEvents converts streamEvent to Event. If device is non-nil and
	// matches the streamevent.transactionID device then the transaction ID gets
	// added to the unsigned section of the output event.
	StreamEventsToEvents(ctx context.Context, device *userapi.Device, in []types.StreamEvent, rsAPI api.SyncRoomserverAPI) []*rstypes.HeaderedEvent
	// SendToDeviceUpdatesForSync returns a list of send-to-device updates. It returns the
	// relevant events within the given ranges for the supplied user ID and device ID.
	SendToDeviceUpdatesForSync(ctx context.Context, userID, deviceID string, from, to types.StreamPosition) (pos types.StreamPosition, events []types.SendToDeviceEvent, err error)
	// GetRoomReceipts gets all receipts for a given roomID
	GetRoomReceipts(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) ([]types.OutputReceiptEvent, error)
	SelectContextEvent(ctx context.Context, roomID, eventID string) (int, rstypes.HeaderedEvent, error)
	SelectContextBeforeEvent(ctx context.Context, id int, roomID string, filter *synctypes.RoomEventFilter) ([]*rstypes.HeaderedEvent, error)
	SelectContextAfterEvent(ctx context.Context, id int, roomID string, filter *synctypes.RoomEventFilter) (int, []*rstypes.HeaderedEvent, error)
	StreamToTopologicalPosition(ctx context.Context, roomID string, streamPos types.StreamPosition, backwardOrdering bool) (types.TopologyToken, error)
	IgnoresForUser(ctx context.Context, userID string) (*types.IgnoredUsers, error)
	// SelectMembershipForUser returns the membership of the user before and including the given position. If no membership can be found
	// returns "leave", the topological position and no error. If an error occurs, other than sql.ErrNoRows, returns that and an empty
	// string as the membership.
	SelectMembershipForUser(ctx context.Context, roomID, userID string, pos int64) (membership string, topologicalPos int, err error)
	// getUserUnreadNotificationCountsForRooms returns the unread notifications for the given rooms
	GetUserUnreadNotificationCountsForRooms(ctx context.Context, userID string, roomIDs map[string]string) (map[string]*eventutil.NotificationData, error)
	GetPresences(ctx context.Context, userID []string) ([]*types.PresenceInternal, error)
	PresenceAfter(ctx context.Context, after types.StreamPosition, filter synctypes.EventFilter) (map[string]*types.PresenceInternal, error)
	RelationsFor(ctx context.Context, roomID, eventID, relType, eventType string, from, to types.StreamPosition, backwards bool, limit int) (events []types.StreamEvent, prevBatch, nextBatch string, err error)
}

type Database interface {
	Presence
	Notifications

	NewDatabaseSnapshot(ctx context.Context) (*shared.DatabaseTransaction, error)
	NewDatabaseTransaction(ctx context.Context) (*shared.DatabaseTransaction, error)

	// Events lookups a list of event by their event ID.
	// Returns a list of events matching the requested IDs found in the database.
	// If an event is not found in the database then it will be omitted from the list.
	// Returns an error if there was a problem talking with the database.
	// Does not include any transaction IDs in the returned events.
	Events(ctx context.Context, eventIDs []string) ([]*rstypes.HeaderedEvent, error)
	// WriteEvent into the database. It is not safe to call this function from multiple goroutines, as it would create races
	// when generating the sync stream position for this event. Returns the sync stream position for the inserted event.
	// Returns an error if there was a problem inserting this event.
	WriteEvent(ctx context.Context, ev *rstypes.HeaderedEvent, addStateEvents []*rstypes.HeaderedEvent,
		addStateEventIDs []string, removeStateEventIDs []string, transactionID *api.TransactionID, excludeFromSync bool,
		historyVisibility gomatrixserverlib.HistoryVisibility,
	) (types.StreamPosition, error)
	// PurgeRoomState completely purges room state from the sync API. This is done when
	// receiving an output event that completely resets the state.
	PurgeRoomState(ctx context.Context, roomID string) error
	// PurgeRoom entirely eliminates a room from the sync API, timeline, state and all.
	PurgeRoom(ctx context.Context, roomID string) error
	// UpsertAccountData keeps track of new or updated account data, by saving the type
	// of the new/updated data, and the user ID and room ID the data is related to (empty)
	// room ID means the data isn't specific to any room)
	// If no data with the given type, user ID and room ID exists in the database,
	// creates a new row, else update the existing one
	// Returns an error if there was an issue with the upsert
	UpsertAccountData(ctx context.Context, userID, roomID, dataType string) (types.StreamPosition, error)
	// AddInviteEvent stores a new invite event for a user.
	// If the invite was successfully stored this returns the stream ID it was stored at.
	// Returns an error if there was a problem communicating with the database.
	AddInviteEvent(ctx context.Context, inviteEvent *rstypes.HeaderedEvent) (types.StreamPosition, error)
	// RetireInviteEvent removes an old invite event from the database. Returns the new position of the retired invite.
	// Returns an error if there was a problem communicating with the database.
	RetireInviteEvent(ctx context.Context, inviteEventID string) (types.StreamPosition, error)
	// AddPeek adds a new peek to our DB for a given room by a given user's device.
	// Returns an error if there was a problem communicating with the database.
	AddPeek(ctx context.Context, RoomID, UserID, DeviceID string) (types.StreamPosition, error)
	// DeletePeek removes an existing peek from the database for a given room by a user's device.
	// Returns an error if there was a problem communicating with the database.
	DeletePeek(ctx context.Context, roomID, userID, deviceID string) (sp types.StreamPosition, err error)
	// DeletePeek deletes all peeks for a given room by a given user
	// Returns an error if there was a problem communicating with the database.
	DeletePeeks(ctx context.Context, RoomID, UserID string) (types.StreamPosition, error)
	// StoreNewSendForDeviceMessage stores a new send-to-device event for a user's device.
	StoreNewSendForDeviceMessage(ctx context.Context, userID, deviceID string, event gomatrixserverlib.SendToDeviceEvent) (types.StreamPosition, error)
	// CleanSendToDeviceUpdates removes all send-to-device messages BEFORE the specified
	// from position, preventing the send-to-device table from growing indefinitely.
	CleanSendToDeviceUpdates(ctx context.Context, userID, deviceID string, before types.StreamPosition) (err error)
	// GetFilter looks up the filter associated with a given local user and filter ID
	// and populates the target filter. Otherwise returns an error if no such filter exists
	// or if there was an error talking to the database.
	GetFilter(ctx context.Context, target *synctypes.Filter, localpart string, filterID string) error
	// PutFilter puts the passed filter into the database.
	// Returns the filterID as a string. Otherwise returns an error if something
	// goes wrong.
	PutFilter(ctx context.Context, localpart string, filter *synctypes.Filter) (string, error)
	// RedactEvent wipes an event in the database and sets the unsigned.redacted_because key to the redaction event
	RedactEvent(ctx context.Context, redactedEventID string, redactedBecause *rstypes.HeaderedEvent, querier api.QuerySenderIDAPI) error
	// StoreReceipt stores new receipt events
	StoreReceipt(ctx context.Context, roomId, receiptType, userId, eventId string, timestamp spec.Timestamp) (pos types.StreamPosition, err error)
	UpdateIgnoresForUser(ctx context.Context, userID string, ignores *types.IgnoredUsers) error
	ReIndex(ctx context.Context, limit, afterID int64) (map[int64]rstypes.HeaderedEvent, error)
	UpdateRelations(ctx context.Context, event *rstypes.HeaderedEvent) error
	RedactRelations(ctx context.Context, roomID, redactedEventID string) error
	SelectMemberships(
		ctx context.Context,
		roomID string, pos types.TopologyToken,
		membership, notMembership *string,
	) (eventIDs []string, err error)
}

type Presence interface {
	GetPresences(ctx context.Context, userIDs []string) ([]*types.PresenceInternal, error)
	UpdatePresence(ctx context.Context, userID string, presence types.Presence, statusMsg *string, lastActiveTS spec.Timestamp, fromSync bool) (types.StreamPosition, error)
}

type SharedUsers interface {
	// SharedUsers returns a subset of otherUserIDs that share a room with userID.
	SharedUsers(ctx context.Context, userID string, otherUserIDs []string) ([]string, error)
}

type Notifications interface {
	// UpsertRoomUnreadNotificationCounts updates the notification statistics about a (user, room) key.
	UpsertRoomUnreadNotificationCounts(ctx context.Context, userID, roomID string, notificationCount, highlightCount int) (types.StreamPosition, error)
}
