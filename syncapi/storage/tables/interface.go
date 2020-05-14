package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type AccountData interface {
	InsertAccountData(ctx context.Context, txn *sql.Tx, userID, roomID, dataType string) (pos types.StreamPosition, err error)
	// SelectAccountDataInRange returns a map of room ID to a list of `dataType`.
	SelectAccountDataInRange(ctx context.Context, userID string, r types.Range, accountDataEventFilter *gomatrixserverlib.EventFilter) (data map[string][]string, err error)
	SelectMaxAccountDataID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}

type Invites interface {
	InsertInviteEvent(ctx context.Context, txn *sql.Tx, inviteEvent gomatrixserverlib.HeaderedEvent) (streamPos types.StreamPosition, err error)
	DeleteInviteEvent(ctx context.Context, inviteEventID string) error
	// SelectInviteEventsInRange returns a map of room ID to invite events.
	SelectInviteEventsInRange(ctx context.Context, txn *sql.Tx, targetUserID string, r types.Range) (map[string]gomatrixserverlib.HeaderedEvent, error)
	SelectMaxInviteID(ctx context.Context, txn *sql.Tx) (id int64, err error)
}

type Events interface {
	SelectStateInRange(ctx context.Context, txn *sql.Tx, r types.Range, stateFilter *gomatrixserverlib.StateFilter) (map[string]map[string]bool, map[string]types.StreamEvent, error)
	SelectMaxEventID(ctx context.Context, txn *sql.Tx) (id int64, err error)
	InsertEvent(ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.HeaderedEvent, addState, removeState []string, transactionID *api.TransactionID, excludeFromSync bool) (streamPos types.StreamPosition, err error)
	// SelectRecentEvents returns events between the two stream positions: exclusive of low and inclusive of high.
	// If onlySyncEvents has a value of true, only returns the events that aren't marked as to exclude from sync.
	// Returns up to `limit` events.
	SelectRecentEvents(ctx context.Context, txn *sql.Tx, roomID string, r types.Range, limit int, chronologicalOrder bool, onlySyncEvents bool) ([]types.StreamEvent, error)
	// SelectEarlyEvents returns the earliest events in the given room.
	SelectEarlyEvents(ctx context.Context, txn *sql.Tx, roomID string, r types.Range, limit int) ([]types.StreamEvent, error)
	SelectEvents(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]types.StreamEvent, error)
}

// Topology keeps track of the depths and stream positions for all events.
// These positions are used as types.TopologyToken when backfilling events locally.
type Topology interface {
	// InsertEventInTopology inserts the given event in the room's topology, based on the event's depth.
	// `pos` is the stream position of this event in the events table, and is used to order events which have the same depth.
	InsertEventInTopology(ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.HeaderedEvent, pos types.StreamPosition) (err error)
	// SelectEventIDsInRange selects the IDs of events whose depths are within a given range in a given room's topological order.
	// Events with `minDepth` are *exclusive*, as is the event which has exactly `minDepth`,`maxStreamPos`.
	// `maxStreamPos` is only used when events have the same depth as `maxDepth`, which results in events less than `maxStreamPos` being returned.
	// Returns an empty slice if no events match the given range.
	SelectEventIDsInRange(ctx context.Context, txn *sql.Tx, roomID string, minDepth, maxDepth, maxStreamPos types.StreamPosition, limit int, chronologicalOrder bool) (eventIDs []string, err error)
	// SelectPositionInTopology returns the depth and stream position of a given event in the topology of the room it belongs to.
	SelectPositionInTopology(ctx context.Context, txn *sql.Tx, eventID string) (depth, spos types.StreamPosition, err error)
	// SelectMaxPositionInTopology returns the event which has the highest depth, and if there are multiple, the event with the highest stream position.
	SelectMaxPositionInTopology(ctx context.Context, txn *sql.Tx, roomID string) (depth types.StreamPosition, spos types.StreamPosition, err error)
}

type CurrentRoomState interface {
	SelectStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error)
	SelectEventsWithEventIDs(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]types.StreamEvent, error)
	UpsertRoomState(ctx context.Context, txn *sql.Tx, event gomatrixserverlib.HeaderedEvent, membership *string, addedAt types.StreamPosition) error
	DeleteRoomStateByEventID(ctx context.Context, txn *sql.Tx, eventID string) error
	// SelectCurrentState returns all the current state events for the given room.
	SelectCurrentState(ctx context.Context, txn *sql.Tx, roomID string, stateFilter *gomatrixserverlib.StateFilter) ([]gomatrixserverlib.HeaderedEvent, error)
	// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
	SelectRoomIDsWithMembership(ctx context.Context, txn *sql.Tx, userID string, membership string) ([]string, error)
	// SelectJoinedUsers returns a map of room ID to a list of joined user IDs.
	SelectJoinedUsers(ctx context.Context) (map[string][]string, error)
}

// BackwardsExtremities keeps track of backwards extremities for a room.
// Backwards extremities are the earliest (DAG-wise) known events which we have
// the entire event JSON. These event IDs are used in federation requests to fetch
// even earlier events.
//
// We persist the previous event IDs as well, one per row, so when we do fetch even
// earlier events we can simply delete rows which referenced it. Consider the graph:
//        A
//        |   Event C has 1 prev_event ID: A.
//    B   C
//    |___|   Event D has 2 prev_event IDs: B and C.
//      |
//      D
// The earliest known event we have is D, so this table has 2 rows.
// A backfill request gives us C but not B. We delete rows where prev_event=C. This
// still means that D is a backwards extremity as we do not have event B. However, event
// C is *also* a backwards extremity at this point as we do not have event A. Later,
// when we fetch event B, we delete rows where prev_event=B. This then removes D as
// a backwards extremity because there are no more rows with event_id=B.
type BackwardsExtremities interface {
	// InsertsBackwardExtremity inserts a new backwards extremity.
	InsertsBackwardExtremity(ctx context.Context, txn *sql.Tx, roomID, eventID string, prevEventID string) (err error)
	// SelectBackwardExtremitiesForRoom retrieves all backwards extremities for the room.
	SelectBackwardExtremitiesForRoom(ctx context.Context, roomID string) (eventIDs []string, err error)
	// DeleteBackwardExtremity removes a backwards extremity for a room, if one existed.
	DeleteBackwardExtremity(ctx context.Context, txn *sql.Tx, roomID, knownEventID string) (err error)
}
