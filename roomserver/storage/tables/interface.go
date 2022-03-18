package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
)

type EventJSONPair struct {
	EventNID    types.EventNID
	RoomVersion gomatrixserverlib.RoomVersion
	EventJSON   []byte
}

type EventJSON interface {
	// Insert the event JSON. On conflict, replace the event JSON with the new value (for redactions).
	InsertEventJSON(ctx context.Context, tx *sql.Tx, eventNID types.EventNID, eventJSON []byte) error
	BulkSelectEventJSON(ctx context.Context, tx *sql.Tx, eventNIDs []types.EventNID) ([]EventJSONPair, error)
}

type EventTypes interface {
	InsertEventTypeNID(ctx context.Context, tx *sql.Tx, eventType string) (types.EventTypeNID, error)
	SelectEventTypeNID(ctx context.Context, tx *sql.Tx, eventType string) (types.EventTypeNID, error)
	BulkSelectEventTypeNID(ctx context.Context, txn *sql.Tx, eventTypes []string) (map[string]types.EventTypeNID, error)
}

type EventStateKeys interface {
	InsertEventStateKeyNID(ctx context.Context, txn *sql.Tx, eventStateKey string) (types.EventStateKeyNID, error)
	SelectEventStateKeyNID(ctx context.Context, txn *sql.Tx, eventStateKey string) (types.EventStateKeyNID, error)
	BulkSelectEventStateKeyNID(ctx context.Context, txn *sql.Tx, eventStateKeys []string) (map[string]types.EventStateKeyNID, error)
	BulkSelectEventStateKey(ctx context.Context, txn *sql.Tx, eventStateKeyNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error)
}

type Events interface {
	InsertEvent(
		ctx context.Context, txn *sql.Tx, i types.RoomNID, j types.EventTypeNID, k types.EventStateKeyNID, eventID string,
		referenceSHA256 []byte, authEventNIDs []types.EventNID, depth int64, isRejected bool,
	) (types.EventNID, types.StateSnapshotNID, error)
	SelectEvent(ctx context.Context, txn *sql.Tx, eventID string) (types.EventNID, types.StateSnapshotNID, error)
	// bulkSelectStateEventByID lookups a list of state events by event ID.
	// If any of the requested events are missing from the database it returns a types.MissingEventError
	BulkSelectStateEventByID(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]types.StateEntry, error)
	BulkSelectStateEventByNID(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID, stateKeyTuples []types.StateKeyTuple) ([]types.StateEntry, error)
	// BulkSelectStateAtEventByID lookups the state at a list of events by event ID.
	// If any of the requested events are missing from the database it returns a types.MissingEventError.
	// If we do not have the state for any of the requested events it returns a types.MissingEventError.
	BulkSelectStateAtEventByID(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]types.StateAtEvent, error)
	UpdateEventState(ctx context.Context, txn *sql.Tx, eventNID types.EventNID, stateNID types.StateSnapshotNID) error
	SelectEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) (sentToOutput bool, err error)
	UpdateEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) error
	SelectEventID(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) (eventID string, err error)
	BulkSelectStateAtEventAndReference(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) ([]types.StateAtEventAndReference, error)
	BulkSelectEventReference(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) ([]gomatrixserverlib.EventReference, error)
	// BulkSelectEventID returns a map from numeric event ID to string event ID.
	BulkSelectEventID(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (map[types.EventNID]string, error)
	// BulkSelectEventNIDs returns a map from string event ID to numeric event ID.
	// If an event ID is not in the database then it is omitted from the map.
	BulkSelectEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string) (map[string]types.EventNID, error)
	BulkSelectUnsentEventNID(ctx context.Context, txn *sql.Tx, eventIDs []string) (map[string]types.EventNID, error)
	SelectMaxEventDepth(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (int64, error)
	SelectRoomNIDsForEventNIDs(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (roomNIDs map[types.EventNID]types.RoomNID, err error)
}

type Rooms interface {
	InsertRoomNID(ctx context.Context, txn *sql.Tx, roomID string, roomVersion gomatrixserverlib.RoomVersion) (types.RoomNID, error)
	SelectRoomNID(ctx context.Context, txn *sql.Tx, roomID string) (types.RoomNID, error)
	SelectLatestEventNIDs(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID) ([]types.EventNID, types.StateSnapshotNID, error)
	SelectLatestEventsNIDsForUpdate(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID) ([]types.EventNID, types.EventNID, types.StateSnapshotNID, error)
	UpdateLatestEventNIDs(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, eventNIDs []types.EventNID, lastEventSentNID types.EventNID, stateSnapshotNID types.StateSnapshotNID) error
	SelectRoomVersionsForRoomNIDs(ctx context.Context, txn *sql.Tx, roomNID []types.RoomNID) (map[types.RoomNID]gomatrixserverlib.RoomVersion, error)
	SelectRoomInfo(ctx context.Context, txn *sql.Tx, roomID string) (*types.RoomInfo, error)
	SelectRoomIDs(ctx context.Context, txn *sql.Tx) ([]string, error)
	BulkSelectRoomIDs(ctx context.Context, txn *sql.Tx, roomNIDs []types.RoomNID) ([]string, error)
	BulkSelectRoomNIDs(ctx context.Context, txn *sql.Tx, roomIDs []string) ([]types.RoomNID, error)
}

type StateSnapshot interface {
	InsertState(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, stateBlockNIDs types.StateBlockNIDs) (stateNID types.StateSnapshotNID, err error)
	BulkSelectStateBlockNIDs(ctx context.Context, txn *sql.Tx, stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error)
}

type StateBlock interface {
	BulkInsertStateData(ctx context.Context, txn *sql.Tx, entries types.StateEntries) (types.StateBlockNID, error)
	BulkSelectStateBlockEntries(ctx context.Context, txn *sql.Tx, stateBlockNIDs types.StateBlockNIDs) ([][]types.EventNID, error)
	//BulkSelectFilteredStateBlockEntries(ctx context.Context, stateBlockNIDs []types.StateBlockNID, stateKeyTuples []types.StateKeyTuple) ([]types.StateEntryList, error)
}

type RoomAliases interface {
	InsertRoomAlias(ctx context.Context, txn *sql.Tx, alias string, roomID string, creatorUserID string) (err error)
	SelectRoomIDFromAlias(ctx context.Context, txn *sql.Tx, alias string) (roomID string, err error)
	SelectAliasesFromRoomID(ctx context.Context, txn *sql.Tx, roomID string) ([]string, error)
	SelectCreatorIDFromAlias(ctx context.Context, txn *sql.Tx, alias string) (creatorID string, err error)
	DeleteRoomAlias(ctx context.Context, txn *sql.Tx, alias string) (err error)
}

type PreviousEvents interface {
	InsertPreviousEvent(ctx context.Context, txn *sql.Tx, previousEventID string, previousEventReferenceSHA256 []byte, eventNID types.EventNID) error
	// Check if the event reference exists
	// Returns sql.ErrNoRows if the event reference doesn't exist.
	SelectPreviousEventExists(ctx context.Context, txn *sql.Tx, eventID string, eventReferenceSHA256 []byte) error
}

type Invites interface {
	InsertInviteEvent(ctx context.Context, txn *sql.Tx, inviteEventID string, roomNID types.RoomNID, targetUserNID, senderUserNID types.EventStateKeyNID, inviteEventJSON []byte) (bool, error)
	UpdateInviteRetired(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID) ([]string, error)
	// SelectInviteActiveForUserInRoom returns a list of sender state key NIDs and invite event IDs matching those nids.
	SelectInviteActiveForUserInRoom(ctx context.Context, txn *sql.Tx, targetUserNID types.EventStateKeyNID, roomNID types.RoomNID) ([]types.EventStateKeyNID, []string, error)
}

type MembershipState int64

const (
	MembershipStateLeaveOrBan MembershipState = 1
	MembershipStateInvite     MembershipState = 2
	MembershipStateJoin       MembershipState = 3
	MembershipStateKnock      MembershipState = 4
)

type Membership interface {
	InsertMembership(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID, localTarget bool) error
	SelectMembershipForUpdate(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID) (MembershipState, error)
	SelectMembershipFromRoomAndTarget(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID) (types.EventNID, MembershipState, bool, error)
	SelectMembershipsFromRoom(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, localOnly bool) (eventNIDs []types.EventNID, err error)
	SelectMembershipsFromRoomAndMembership(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, membership MembershipState, localOnly bool) (eventNIDs []types.EventNID, err error)
	UpdateMembership(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID, senderUserNID types.EventStateKeyNID, membership MembershipState, eventNID types.EventNID, forgotten bool) (bool, error)
	SelectRoomsWithMembership(ctx context.Context, txn *sql.Tx, userID types.EventStateKeyNID, membershipState MembershipState) ([]types.RoomNID, error)
	// SelectJoinedUsersSetForRooms returns how many times each of the given users appears across the given rooms.
	SelectJoinedUsersSetForRooms(ctx context.Context, txn *sql.Tx, roomNIDs []types.RoomNID, userNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]int, error)
	SelectKnownUsers(ctx context.Context, txn *sql.Tx, userID types.EventStateKeyNID, searchString string, limit int) ([]string, error)
	UpdateForgetMembership(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID, forget bool) error
	SelectLocalServerInRoom(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID) (bool, error)
	SelectServerInRoom(ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, serverName gomatrixserverlib.ServerName) (bool, error)
}

type Published interface {
	UpsertRoomPublished(ctx context.Context, txn *sql.Tx, roomID string, published bool) (err error)
	SelectPublishedFromRoomID(ctx context.Context, txn *sql.Tx, roomID string) (published bool, err error)
	SelectAllPublishedRooms(ctx context.Context, txn *sql.Tx, published bool) ([]string, error)
}

type RedactionInfo struct {
	// whether this redaction is validated (we have both events)
	Validated bool
	// the ID of the event being redacted
	RedactsEventID string
	// the ID of the redaction event
	RedactionEventID string
}

type Redactions interface {
	InsertRedaction(ctx context.Context, txn *sql.Tx, info RedactionInfo) error
	// SelectRedactionInfoByRedactionEventID returns the redaction info for the given redaction event ID, or nil if there is no match.
	SelectRedactionInfoByRedactionEventID(ctx context.Context, txn *sql.Tx, redactionEventID string) (*RedactionInfo, error)
	// SelectRedactionInfoByEventBeingRedacted returns the redaction info for the given redacted event ID, or nil if there is no match.
	SelectRedactionInfoByEventBeingRedacted(ctx context.Context, txn *sql.Tx, eventID string) (*RedactionInfo, error)
	// Mark this redaction event as having been validated. This means we have both sides of the redaction and have
	// successfully redacted the event JSON.
	MarkRedactionValidated(ctx context.Context, txn *sql.Tx, redactionEventID string, validated bool) error
}

// StrippedEvent represents a stripped event for returning extracted content values.
type StrippedEvent struct {
	RoomID       string
	EventType    string
	StateKey     string
	ContentValue string
}

// ExtractContentValue from the given state event. For example, given an m.room.name event with:
//    content: { name: "Foo" }
// this returns "Foo".
func ExtractContentValue(ev *gomatrixserverlib.HeaderedEvent) string {
	content := ev.Content()
	key := ""
	switch ev.Type() {
	case gomatrixserverlib.MRoomCreate:
		key = "creator"
	case gomatrixserverlib.MRoomCanonicalAlias:
		key = "alias"
	case gomatrixserverlib.MRoomHistoryVisibility:
		key = "history_visibility"
	case gomatrixserverlib.MRoomJoinRules:
		key = "join_rule"
	case gomatrixserverlib.MRoomMember:
		key = "membership"
	case gomatrixserverlib.MRoomName:
		key = "name"
	case "m.room.avatar":
		key = "url"
	case "m.room.topic":
		key = "topic"
	case "m.room.guest_access":
		key = "guest_access"
	}
	result := gjson.GetBytes(content, key)
	if !result.Exists() {
		return ""
	}
	// this returns the empty string if this is not a string type
	return result.Str
}
