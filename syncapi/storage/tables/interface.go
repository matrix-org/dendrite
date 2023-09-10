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

package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/syncapi/types"
)

type AccountData interface {
	InsertAccountData(ctx context.Context, txn *sql.Tx, userID, roomID, dataType string) (pos types.StreamPosition, err error)
	// SelectAccountDataInRange returns a map of room ID to a list of `dataType`.
	SelectAccountDataInRange(ctx context.Context, txn *sql.Tx, userID string, r types.Range, accountDataEventFilter *synctypes.EventFilter) (data map[string][]string, pos types.StreamPosition, err error)
	SelectMaxAccountDataID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}

type Invites interface {
	InsertInviteEvent(ctx context.Context, txn *sql.Tx, inviteEvent *rstypes.HeaderedEvent) (streamPos types.StreamPosition, err error)
	DeleteInviteEvent(ctx context.Context, txn *sql.Tx, inviteEventID string) (types.StreamPosition, error)
	// SelectInviteEventsInRange returns a map of room ID to invite events. If multiple invite/retired invites exist in the given range, return the latest value
	// for the room.
	SelectInviteEventsInRange(ctx context.Context, txn *sql.Tx, targetUserID string, r types.Range) (invites map[string]*rstypes.HeaderedEvent, retired map[string]*rstypes.HeaderedEvent, maxID types.StreamPosition, err error)
	SelectMaxInviteID(ctx context.Context, txn *sql.Tx) (id int64, err error)
	PurgeInvites(ctx context.Context, txn *sql.Tx, roomID string) error
}

type Peeks interface {
	InsertPeek(ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string) (streamPos types.StreamPosition, err error)
	DeletePeek(ctx context.Context, txn *sql.Tx, roomID, userID, deviceID string) (streamPos types.StreamPosition, err error)
	DeletePeeks(ctx context.Context, txn *sql.Tx, roomID, userID string) (streamPos types.StreamPosition, err error)
	SelectPeeksInRange(ctxt context.Context, txn *sql.Tx, userID, deviceID string, r types.Range) (peeks []types.Peek, err error)
	SelectPeekingDevices(ctxt context.Context, txn *sql.Tx) (peekingDevices map[string][]types.PeekingDevice, err error)
	SelectMaxPeekID(ctx context.Context, txn *sql.Tx) (id int64, err error)
	PurgePeeks(ctx context.Context, txn *sql.Tx, roomID string) error
}

type Events interface {
	SelectStateInRange(ctx context.Context, txn *sql.Tx, r types.Range, stateFilter *synctypes.StateFilter, roomIDs []string) (map[string]map[string]bool, map[string]types.StreamEvent, error)
	SelectMaxEventID(ctx context.Context, txn *sql.Tx) (id int64, err error)
	InsertEvent(
		ctx context.Context, txn *sql.Tx,
		event *rstypes.HeaderedEvent,
		addState, removeState []string,
		transactionID *api.TransactionID,
		excludeFromSync bool,
		historyVisibility gomatrixserverlib.HistoryVisibility,
	) (streamPos types.StreamPosition, err error)
	// SelectRecentEvents returns events between the two stream positions: exclusive of low and inclusive of high.
	// If onlySyncEvents has a value of true, only returns the events that aren't marked as to exclude from sync.
	// Returns up to `limit` events. Returns `limited=true` if there are more events in this range but we hit the `limit`.
	SelectRecentEvents(ctx context.Context, txn *sql.Tx, roomIDs []string, r types.Range, eventFilter *synctypes.RoomEventFilter, chronologicalOrder bool, onlySyncEvents bool) (map[string]types.RecentEvents, error)
	SelectEvents(ctx context.Context, txn *sql.Tx, eventIDs []string, filter *synctypes.RoomEventFilter, preserveOrder bool) ([]types.StreamEvent, error)
	UpdateEventJSON(ctx context.Context, txn *sql.Tx, event *rstypes.HeaderedEvent) error
	// DeleteEventsForRoom removes all event information for a room. This should only be done when removing the room entirely.
	DeleteEventsForRoom(ctx context.Context, txn *sql.Tx, roomID string) (err error)

	SelectContextEvent(ctx context.Context, txn *sql.Tx, roomID, eventID string) (int, rstypes.HeaderedEvent, error)
	SelectContextBeforeEvent(ctx context.Context, txn *sql.Tx, id int, roomID string, filter *synctypes.RoomEventFilter) ([]*rstypes.HeaderedEvent, error)
	SelectContextAfterEvent(ctx context.Context, txn *sql.Tx, id int, roomID string, filter *synctypes.RoomEventFilter) (int, []*rstypes.HeaderedEvent, error)

	PurgeEvents(ctx context.Context, txn *sql.Tx, roomID string) error
	ReIndex(ctx context.Context, txn *sql.Tx, limit, offset int64, types []string) (map[int64]rstypes.HeaderedEvent, error)
}

// Topology keeps track of the depths and stream positions for all events.
// These positions are used as types.TopologyToken when backfilling events locally.
type Topology interface {
	// InsertEventInTopology inserts the given event in the room's topology, based on the event's depth.
	// `pos` is the stream position of this event in the events table, and is used to order events which have the same depth.
	InsertEventInTopology(ctx context.Context, txn *sql.Tx, event *rstypes.HeaderedEvent, pos types.StreamPosition) (topoPos types.StreamPosition, err error)
	// SelectEventIDsInRange selects the IDs and the topological position of events whose depths are within a given range in a given room's topological order.
	// Events with `minDepth` are *exclusive*, as is the event which has exactly `minDepth`,`maxStreamPos`. Returns the eventIDs and start/end topological tokens.
	// `maxStreamPos` is only used when events have the same depth as `maxDepth`, which results in events less than `maxStreamPos` being returned.
	// Returns an empty slice if no events match the given range.
	SelectEventIDsInRange(ctx context.Context, txn *sql.Tx, roomID string, minDepth, maxDepth, maxStreamPos types.StreamPosition, limit int, chronologicalOrder bool) (eventIDs []string, start, end types.TopologyToken, err error)
	// SelectPositionInTopology returns the depth and stream position of a given event in the topology of the room it belongs to.
	SelectPositionInTopology(ctx context.Context, txn *sql.Tx, eventID string) (depth, spos types.StreamPosition, err error)
	// SelectStreamToTopologicalPosition converts a stream position to a topological position by finding the nearest topological position in the room.
	SelectStreamToTopologicalPosition(ctx context.Context, txn *sql.Tx, roomID string, streamPos types.StreamPosition, forward bool) (topoPos types.StreamPosition, err error)
	PurgeEventsTopology(ctx context.Context, txn *sql.Tx, roomID string) error
}

type CurrentRoomState interface {
	SelectStateEvent(ctx context.Context, txn *sql.Tx, roomID, evType, stateKey string) (*rstypes.HeaderedEvent, error)
	SelectEventsWithEventIDs(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]types.StreamEvent, error)
	UpsertRoomState(ctx context.Context, txn *sql.Tx, event *rstypes.HeaderedEvent, membership *string, addedAt types.StreamPosition) error
	DeleteRoomStateByEventID(ctx context.Context, txn *sql.Tx, eventID string) error
	DeleteRoomStateForRoom(ctx context.Context, txn *sql.Tx, roomID string) error
	// SelectCurrentState returns all the current state events for the given room.
	SelectCurrentState(ctx context.Context, txn *sql.Tx, roomID string, stateFilter *synctypes.StateFilter, excludeEventIDs []string) ([]*rstypes.HeaderedEvent, error)
	// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
	SelectRoomIDsWithMembership(ctx context.Context, txn *sql.Tx, userID string, membership string) ([]string, error)
	// SelectRoomIDsWithAnyMembership returns a map of all memberships for the given user.
	SelectRoomIDsWithAnyMembership(ctx context.Context, txn *sql.Tx, userID string) (map[string]string, error)
	// SelectJoinedUsers returns a map of room ID to a list of joined user IDs.
	SelectJoinedUsers(ctx context.Context, txn *sql.Tx) (map[string][]string, error)
	// SelectJoinedUsersInRoom returns a map of room ID to a list of joined user IDs for a given room.
	SelectJoinedUsersInRoom(ctx context.Context, txn *sql.Tx, roomIDs []string) (map[string][]string, error)
	// SelectSharedUsers returns a subset of otherUserIDs that share a room with userID.
	SelectSharedUsers(ctx context.Context, txn *sql.Tx, userID string, otherUserIDs []string) ([]string, error)

	SelectRoomHeroes(ctx context.Context, txn *sql.Tx, roomID, excludeUserID string, memberships []string) ([]string, error)
	SelectMembershipCount(ctx context.Context, txn *sql.Tx, roomID, membership string) (int, error)
}

// BackwardsExtremities keeps track of backwards extremities for a room.
// Backwards extremities are the earliest (DAG-wise) known events which we have
// the entire event JSON. These event IDs are used in federation requests to fetch
// even earlier events.
//
// We persist the previous event IDs as well, one per row, so when we do fetch even
// earlier events we can simply delete rows which referenced it. Consider the graph:
//
//	    A
//	    |   Event C has 1 prev_event ID: A.
//	B   C
//	|___|   Event D has 2 prev_event IDs: B and C.
//	  |
//	  D
//
// The earliest known event we have is D, so this table has 2 rows.
// A backfill request gives us C but not B. We delete rows where prev_event=C. This
// still means that D is a backwards extremity as we do not have event B. However, event
// C is *also* a backwards extremity at this point as we do not have event A. Later,
// when we fetch event B, we delete rows where prev_event=B. This then removes D as
// a backwards extremity because there are no more rows with event_id=B.
type BackwardsExtremities interface {
	// InsertsBackwardExtremity inserts a new backwards extremity.
	InsertsBackwardExtremity(ctx context.Context, txn *sql.Tx, roomID, eventID string, prevEventID string) (err error)
	// SelectBackwardExtremitiesForRoom retrieves all backwards extremities for the room, as a map of event_id to list of prev_event_ids.
	SelectBackwardExtremitiesForRoom(ctx context.Context, txn *sql.Tx, roomID string) (bwExtrems map[string][]string, err error)
	// DeleteBackwardExtremity removes a backwards extremity for a room, if one existed.
	DeleteBackwardExtremity(ctx context.Context, txn *sql.Tx, roomID, knownEventID string) (err error)
	PurgeBackwardExtremities(ctx context.Context, txn *sql.Tx, roomID string) error
}

// SendToDevice tracks send-to-device messages which are sent to individual
// clients. Each message gets inserted into this table at the point that we
// receive it from the EDU server.
//
// We're supposed to try and do our best to deliver send-to-device messages
// once, but the only way that we can really guarantee that they have been
// delivered is if the client successfully requests the next sync as given
// in the next_batch. Each time the device syncs, we will request all of the
// updates that either haven't been sent yet, along with all updates that we
// *have* sent but we haven't confirmed to have been received yet. If it's the
// first time we're sending a given update then we update the table to say
// what the "since" parameter was when we tried to send it.
//
// When the client syncs again, if their "since" parameter is *later* than
// the recorded one, we drop the entry from the DB as it's "sent". If the
// sync parameter isn't later then we will keep including the updates in the
// sync response, as the client is seemingly trying to repeat the same /sync.
type SendToDevice interface {
	InsertSendToDeviceMessage(ctx context.Context, txn *sql.Tx, userID, deviceID, content string) (pos types.StreamPosition, err error)
	SelectSendToDeviceMessages(ctx context.Context, txn *sql.Tx, userID, deviceID string, from, to types.StreamPosition) (lastPos types.StreamPosition, events []types.SendToDeviceEvent, err error)
	DeleteSendToDeviceMessages(ctx context.Context, txn *sql.Tx, userID, deviceID string, from types.StreamPosition) (err error)
	SelectMaxSendToDeviceMessageID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}

type Filter interface {
	SelectFilter(ctx context.Context, txn *sql.Tx, target *synctypes.Filter, localpart string, filterID string) error
	InsertFilter(ctx context.Context, txn *sql.Tx, filter *synctypes.Filter, localpart string) (filterID string, err error)
}

type Receipts interface {
	UpsertReceipt(ctx context.Context, txn *sql.Tx, roomId, receiptType, userId, eventId string, timestamp spec.Timestamp) (pos types.StreamPosition, err error)
	SelectRoomReceiptsAfter(ctx context.Context, txn *sql.Tx, roomIDs []string, streamPos types.StreamPosition) (types.StreamPosition, []types.OutputReceiptEvent, error)
	SelectMaxReceiptID(ctx context.Context, txn *sql.Tx) (id int64, err error)
	PurgeReceipts(ctx context.Context, txn *sql.Tx, roomID string) error
}

type Memberships interface {
	UpsertMembership(ctx context.Context, txn *sql.Tx, event *rstypes.HeaderedEvent, streamPos, topologicalPos types.StreamPosition) error
	SelectMembershipCount(ctx context.Context, txn *sql.Tx, roomID, membership string, pos types.StreamPosition) (count int, err error)
	SelectMembershipForUser(ctx context.Context, txn *sql.Tx, roomID, userID string, pos int64) (membership string, topologicalPos int, err error)
	PurgeMemberships(ctx context.Context, txn *sql.Tx, roomID string) error
	SelectMemberships(
		ctx context.Context, txn *sql.Tx,
		roomID string, pos types.TopologyToken,
		membership, notMembership *string,
	) (eventIDs []string, err error)
}

type NotificationData interface {
	UpsertRoomUnreadCounts(ctx context.Context, txn *sql.Tx, userID, roomID string, notificationCount, highlightCount int) (types.StreamPosition, error)
	SelectUserUnreadCountsForRooms(ctx context.Context, txn *sql.Tx, userID string, roomIDs []string) (map[string]*eventutil.NotificationData, error)
	SelectMaxID(ctx context.Context, txn *sql.Tx) (int64, error)
	PurgeNotificationData(ctx context.Context, txn *sql.Tx, roomID string) error
}

type Ignores interface {
	SelectIgnores(ctx context.Context, txn *sql.Tx, userID string) (*types.IgnoredUsers, error)
	UpsertIgnores(ctx context.Context, txn *sql.Tx, userID string, ignores *types.IgnoredUsers) error
}

type Presence interface {
	UpsertPresence(ctx context.Context, txn *sql.Tx, userID string, statusMsg *string, presence types.Presence, lastActiveTS spec.Timestamp, fromSync bool) (pos types.StreamPosition, err error)
	GetPresenceForUsers(ctx context.Context, txn *sql.Tx, userIDs []string) (presence []*types.PresenceInternal, err error)
	GetMaxPresenceID(ctx context.Context, txn *sql.Tx) (pos types.StreamPosition, err error)
	GetPresenceAfter(ctx context.Context, txn *sql.Tx, after types.StreamPosition, filter synctypes.EventFilter) (presences map[string]*types.PresenceInternal, err error)
}

type Relations interface {
	// Inserts a relation which refers from the child event ID to the event ID in the given room.
	// If the relation already exists then this function will do nothing and return no error.
	InsertRelation(ctx context.Context, txn *sql.Tx, roomID, eventID, childEventID, childEventType, relType string) (err error)
	// Deletes a relation which already exists as the result of an event redaction. If the relation
	// does not exist then this function will do nothing and return no error.
	DeleteRelation(ctx context.Context, txn *sql.Tx, roomID, childEventID string) error
	// SelectRelationsInRange will return relations grouped by relation type within the given range.
	// The map is relType -> []entry. If a relType parameter is specified then the results will only
	// contain relations of that type, otherwise if "" is specified then all relations in the range
	// will be returned, inclusive of the "to" position but excluding the "from" position. The stream
	// position returned is the maximum position of the returned results.
	SelectRelationsInRange(ctx context.Context, txn *sql.Tx, roomID, eventID, relType, eventType string, r types.Range, limit int) (map[string][]types.RelationEntry, types.StreamPosition, error)
	// SelectMaxRelationID returns the maximum ID of all relations, used to determine what the boundaries
	// should be if there are no boundaries supplied (i.e. we want to work backwards but don't have a
	// "from" or want to work forwards and don't have a "to").
	SelectMaxRelationID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}
