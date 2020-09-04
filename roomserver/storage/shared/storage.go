package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	csstables "github.com/matrix-org/dendrite/currentstateserver/storage/tables"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"
)

// Ideally, when we have both events we should redact the event JSON and forget about the redaction, but we currently
// don't because the redaction code is brand new. When we are more certain that redactions don't misbehave or are
// vulnerable to attacks from remote servers (e.g a server bypassing event auth rules shouldn't redact our data)
// then we should flip this to true. This will mean redactions /actually delete information irretrievably/ which
// will be necessary for compliance with the law. Note that downstream components (syncapi) WILL delete information
// in their database on receipt of a redaction. Also note that we still modify the event JSON to set the field
// unsigned.redacted_because - we just don't clear out the content fields yet.
const redactionsArePermanent = true

type Database struct {
	DB                  *sql.DB
	Cache               caching.RoomServerCaches
	Writer              sqlutil.Writer
	EventsTable         tables.Events
	EventJSONTable      tables.EventJSON
	EventTypesTable     tables.EventTypes
	EventStateKeysTable tables.EventStateKeys
	RoomsTable          tables.Rooms
	TransactionsTable   tables.Transactions
	StateSnapshotTable  tables.StateSnapshot
	StateBlockTable     tables.StateBlock
	RoomAliasesTable    tables.RoomAliases
	PrevEventsTable     tables.PreviousEvents
	InvitesTable        tables.Invites
	MembershipTable     tables.Membership
	PublishedTable      tables.Published
	RedactionsTable     tables.Redactions
}

func (d *Database) SupportsConcurrentRoomInputs() bool {
	return true
}

func (d *Database) EventTypeNIDs(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	result := make(map[string]types.EventTypeNID)
	remaining := []string{}
	for _, eventType := range eventTypes {
		if nid, ok := d.Cache.GetRoomServerEventTypeNID(eventType); ok {
			result[eventType] = nid
		} else {
			remaining = append(remaining, eventType)
		}
	}
	if len(remaining) > 0 {
		nids, err := d.EventTypesTable.BulkSelectEventTypeNID(ctx, remaining)
		if err != nil {
			return nil, err
		}
		for eventType, nid := range nids {
			result[eventType] = nid
			d.Cache.StoreRoomServerEventTypeNID(eventType, nid)
		}
	}
	return result, nil
}

func (d *Database) EventStateKeys(
	ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	return d.EventStateKeysTable.BulkSelectEventStateKey(ctx, eventStateKeyNIDs)
}

func (d *Database) EventStateKeyNIDs(
	ctx context.Context, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	result := make(map[string]types.EventStateKeyNID)
	remaining := []string{}
	for _, eventStateKey := range eventStateKeys {
		if nid, ok := d.Cache.GetRoomServerStateKeyNID(eventStateKey); ok {
			result[eventStateKey] = nid
		} else {
			remaining = append(remaining, eventStateKey)
		}
	}
	if len(remaining) > 0 {
		nids, err := d.EventStateKeysTable.BulkSelectEventStateKeyNID(ctx, remaining)
		if err != nil {
			return nil, err
		}
		for eventStateKey, nid := range nids {
			result[eventStateKey] = nid
			d.Cache.StoreRoomServerStateKeyNID(eventStateKey, nid)
		}
	}
	return result, nil
}

func (d *Database) StateEntriesForEventIDs(
	ctx context.Context, eventIDs []string,
) ([]types.StateEntry, error) {
	return d.EventsTable.BulkSelectStateEventByID(ctx, eventIDs)
}

func (d *Database) StateEntriesForTuples(
	ctx context.Context,
	stateBlockNIDs []types.StateBlockNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	return d.StateBlockTable.BulkSelectFilteredStateBlockEntries(
		ctx, stateBlockNIDs, stateKeyTuples,
	)
}

func (d *Database) RoomInfo(ctx context.Context, roomID string) (*types.RoomInfo, error) {
	return d.RoomsTable.SelectRoomInfo(ctx, roomID)
}

func (d *Database) AddState(
	ctx context.Context,
	roomNID types.RoomNID,
	stateBlockNIDs []types.StateBlockNID,
	state []types.StateEntry,
) (stateNID types.StateSnapshotNID, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if len(state) > 0 {
			var stateBlockNID types.StateBlockNID
			stateBlockNID, err = d.StateBlockTable.BulkInsertStateData(ctx, txn, state)
			if err != nil {
				return fmt.Errorf("d.StateBlockTable.BulkInsertStateData: %w", err)
			}
			stateBlockNIDs = append(stateBlockNIDs[:len(stateBlockNIDs):len(stateBlockNIDs)], stateBlockNID)
		}
		stateNID, err = d.StateSnapshotTable.InsertState(ctx, txn, roomNID, stateBlockNIDs)
		if err != nil {
			return fmt.Errorf("d.StateSnapshotTable.InsertState: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("d.Writer.Do: %w", err)
	}
	return
}

func (d *Database) EventNIDs(
	ctx context.Context, eventIDs []string,
) (map[string]types.EventNID, error) {
	return d.EventsTable.BulkSelectEventNID(ctx, eventIDs)
}

func (d *Database) SetState(
	ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.EventsTable.UpdateEventState(ctx, txn, eventNID, stateNID)
	})
}

func (d *Database) StateAtEventIDs(
	ctx context.Context, eventIDs []string,
) ([]types.StateAtEvent, error) {
	return d.EventsTable.BulkSelectStateAtEventByID(ctx, eventIDs)
}

func (d *Database) SnapshotNIDFromEventID(
	ctx context.Context, eventID string,
) (types.StateSnapshotNID, error) {
	_, stateNID, err := d.EventsTable.SelectEvent(ctx, nil, eventID)
	return stateNID, err
}

func (d *Database) EventIDs(
	ctx context.Context, eventNIDs []types.EventNID,
) (map[types.EventNID]string, error) {
	return d.EventsTable.BulkSelectEventID(ctx, eventNIDs)
}

func (d *Database) EventsFromIDs(ctx context.Context, eventIDs []string) ([]types.Event, error) {
	nidMap, err := d.EventNIDs(ctx, eventIDs)
	if err != nil {
		return nil, err
	}

	var nids []types.EventNID
	for _, nid := range nidMap {
		nids = append(nids, nid)
	}

	return d.Events(ctx, nids)
}

func (d *Database) LatestEventIDs(
	ctx context.Context, roomNID types.RoomNID,
) (references []gomatrixserverlib.EventReference, currentStateSnapshotNID types.StateSnapshotNID, depth int64, err error) {
	var eventNIDs []types.EventNID
	eventNIDs, currentStateSnapshotNID, err = d.RoomsTable.SelectLatestEventNIDs(ctx, nil, roomNID)
	if err != nil {
		return
	}
	references, err = d.EventsTable.BulkSelectEventReference(ctx, nil, eventNIDs)
	if err != nil {
		return
	}
	depth, err = d.EventsTable.SelectMaxEventDepth(ctx, nil, eventNIDs)
	if err != nil {
		return
	}
	return
}

func (d *Database) StateBlockNIDs(
	ctx context.Context, stateNIDs []types.StateSnapshotNID,
) ([]types.StateBlockNIDList, error) {
	return d.StateSnapshotTable.BulkSelectStateBlockNIDs(ctx, stateNIDs)
}

func (d *Database) StateEntries(
	ctx context.Context, stateBlockNIDs []types.StateBlockNID,
) ([]types.StateEntryList, error) {
	return d.StateBlockTable.BulkSelectStateBlockEntries(ctx, stateBlockNIDs)
}

func (d *Database) SetRoomAlias(ctx context.Context, alias string, roomID string, creatorUserID string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.RoomAliasesTable.InsertRoomAlias(ctx, txn, alias, roomID, creatorUserID)
	})
}

func (d *Database) GetRoomIDForAlias(ctx context.Context, alias string) (string, error) {
	return d.RoomAliasesTable.SelectRoomIDFromAlias(ctx, alias)
}

func (d *Database) GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error) {
	return d.RoomAliasesTable.SelectAliasesFromRoomID(ctx, roomID)
}

func (d *Database) GetCreatorIDForAlias(
	ctx context.Context, alias string,
) (string, error) {
	return d.RoomAliasesTable.SelectCreatorIDFromAlias(ctx, alias)
}

func (d *Database) RemoveRoomAlias(ctx context.Context, alias string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.RoomAliasesTable.DeleteRoomAlias(ctx, txn, alias)
	})
}

func (d *Database) GetMembership(
	ctx context.Context, roomNID types.RoomNID, requestSenderUserID string,
) (membershipEventNID types.EventNID, stillInRoom bool, err error) {
	var requestSenderUserNID types.EventStateKeyNID
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		requestSenderUserNID, err = d.assignStateKeyNID(ctx, txn, requestSenderUserID)
		return err
	})
	if err != nil {
		return 0, false, fmt.Errorf("d.assignStateKeyNID: %w", err)
	}

	senderMembershipEventNID, senderMembership, err :=
		d.MembershipTable.SelectMembershipFromRoomAndTarget(
			ctx, roomNID, requestSenderUserNID,
		)
	if err == sql.ErrNoRows {
		// The user has never been a member of that room
		return 0, false, nil
	} else if err != nil {
		return
	}

	return senderMembershipEventNID, senderMembership == tables.MembershipStateJoin, nil
}

func (d *Database) GetMembershipEventNIDsForRoom(
	ctx context.Context, roomNID types.RoomNID, joinOnly bool, localOnly bool,
) ([]types.EventNID, error) {
	if joinOnly {
		return d.MembershipTable.SelectMembershipsFromRoomAndMembership(
			ctx, roomNID, tables.MembershipStateJoin, localOnly,
		)
	}

	return d.MembershipTable.SelectMembershipsFromRoom(ctx, roomNID, localOnly)
}

func (d *Database) GetInvitesForUser(
	ctx context.Context,
	roomNID types.RoomNID,
	targetUserNID types.EventStateKeyNID,
) (senderUserIDs []types.EventStateKeyNID, eventIDs []string, err error) {
	return d.InvitesTable.SelectInviteActiveForUserInRoom(ctx, targetUserNID, roomNID)
}

func (d *Database) Events(
	ctx context.Context, eventNIDs []types.EventNID,
) ([]types.Event, error) {
	eventJSONs, err := d.EventJSONTable.BulkSelectEventJSON(ctx, eventNIDs)
	if err != nil {
		return nil, err
	}
	results := make([]types.Event, len(eventJSONs))
	for i, eventJSON := range eventJSONs {
		var roomNID types.RoomNID
		var roomVersion gomatrixserverlib.RoomVersion
		result := &results[i]
		result.EventNID = eventJSON.EventNID
		roomNID, err = d.EventsTable.SelectRoomNIDForEventNID(ctx, eventJSON.EventNID)
		if err != nil {
			return nil, err
		}
		roomVersion, err = d.RoomsTable.SelectRoomVersionForRoomNID(ctx, roomNID)
		if err != nil {
			return nil, err
		}
		result.Event, err = gomatrixserverlib.NewEventFromTrustedJSON(
			eventJSON.EventJSON, false, roomVersion,
		)
		if err != nil {
			return nil, err
		}
	}
	if !redactionsArePermanent {
		d.applyRedactions(results)
	}
	return results, nil
}

func (d *Database) GetTransactionEventID(
	ctx context.Context, transactionID string,
	sessionID int64, userID string,
) (string, error) {
	eventID, err := d.TransactionsTable.SelectTransactionEventID(ctx, transactionID, sessionID, userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return eventID, err
}

func (d *Database) MembershipUpdater(
	ctx context.Context, roomID, targetUserID string,
	targetLocal bool, roomVersion gomatrixserverlib.RoomVersion,
) (*MembershipUpdater, error) {
	txn, err := d.DB.Begin()
	if err != nil {
		return nil, err
	}
	var updater *MembershipUpdater
	_ = d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
		updater, err = NewMembershipUpdater(ctx, d, txn, roomID, targetUserID, targetLocal, roomVersion)
		return nil
	})
	return updater, err
}

func (d *Database) GetLatestEventsForUpdate(
	ctx context.Context, roomInfo types.RoomInfo,
) (*LatestEventsUpdater, error) {
	txn, err := d.DB.Begin()
	if err != nil {
		return nil, err
	}
	var updater *LatestEventsUpdater
	_ = d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
		updater, err = NewLatestEventsUpdater(ctx, d, txn, roomInfo)
		return nil
	})
	return updater, err
}

// nolint:gocyclo
func (d *Database) StoreEvent(
	ctx context.Context, event gomatrixserverlib.Event,
	txnAndSessionID *api.TransactionID, authEventNIDs []types.EventNID,
) (types.RoomNID, types.StateAtEvent, *gomatrixserverlib.Event, string, error) {
	var (
		roomNID          types.RoomNID
		eventTypeNID     types.EventTypeNID
		eventStateKeyNID types.EventStateKeyNID
		eventNID         types.EventNID
		stateNID         types.StateSnapshotNID
		redactionEvent   *gomatrixserverlib.Event
		redactedEventID  string
		err              error
	)

	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if txnAndSessionID != nil {
			if err = d.TransactionsTable.InsertTransaction(
				ctx, txn, txnAndSessionID.TransactionID,
				txnAndSessionID.SessionID, event.Sender(), event.EventID(),
			); err != nil {
				return fmt.Errorf("d.TransactionsTable.InsertTransaction: %w", err)
			}
		}

		// TODO: Here we should aim to have two different code paths for new rooms
		// vs existing ones.

		// Get the default room version. If the client doesn't supply a room_version
		// then we will use our configured default to create the room.
		// https://matrix.org/docs/spec/client_server/r0.6.0#post-matrix-client-r0-createroom
		// Note that the below logic depends on the m.room.create event being the
		// first event that is persisted to the database when creating or joining a
		// room.
		var roomVersion gomatrixserverlib.RoomVersion
		if roomVersion, err = extractRoomVersionFromCreateEvent(event); err != nil {
			return fmt.Errorf("extractRoomVersionFromCreateEvent: %w", err)
		}

		if roomNID, err = d.assignRoomNID(ctx, txn, event.RoomID(), roomVersion); err != nil {
			return fmt.Errorf("d.assignRoomNID: %w", err)
		}

		if eventTypeNID, err = d.assignEventTypeNID(ctx, txn, event.Type()); err != nil {
			return fmt.Errorf("d.assignEventTypeNID: %w", err)
		}

		eventStateKey := event.StateKey()
		// Assigned a numeric ID for the state_key if there is one present.
		// Otherwise set the numeric ID for the state_key to 0.
		if eventStateKey != nil {
			if eventStateKeyNID, err = d.assignStateKeyNID(ctx, txn, *eventStateKey); err != nil {
				return fmt.Errorf("d.assignStateKeyNID: %w", err)
			}
		}

		if eventNID, stateNID, err = d.EventsTable.InsertEvent(
			ctx,
			txn,
			roomNID,
			eventTypeNID,
			eventStateKeyNID,
			event.EventID(),
			event.EventReference().EventSHA256,
			authEventNIDs,
			event.Depth(),
		); err != nil {
			if err == sql.ErrNoRows {
				// We've already inserted the event so select the numeric event ID
				eventNID, stateNID, err = d.EventsTable.SelectEvent(ctx, txn, event.EventID())
			}
			if err != nil {
				return err
			}
		}

		if err = d.EventJSONTable.InsertEventJSON(ctx, txn, eventNID, event.JSON()); err != nil {
			return fmt.Errorf("d.EventJSONTable.InsertEventJSON: %w", err)
		}
		redactionEvent, redactedEventID, err = d.handleRedactions(ctx, txn, eventNID, event)
		return nil
	})
	if err != nil {
		return 0, types.StateAtEvent{}, nil, "", fmt.Errorf("d.Writer.Do: %w", err)
	}

	return roomNID, types.StateAtEvent{
		BeforeStateSnapshotNID: stateNID,
		StateEntry: types.StateEntry{
			StateKeyTuple: types.StateKeyTuple{
				EventTypeNID:     eventTypeNID,
				EventStateKeyNID: eventStateKeyNID,
			},
			EventNID: eventNID,
		},
	}, redactionEvent, redactedEventID, nil
}

func (d *Database) PublishRoom(ctx context.Context, roomID string, publish bool) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.PublishedTable.UpsertRoomPublished(ctx, txn, roomID, publish)
	})
}

func (d *Database) GetPublishedRooms(ctx context.Context) ([]string, error) {
	return d.PublishedTable.SelectAllPublishedRooms(ctx, true)
}

func (d *Database) assignRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (types.RoomNID, error) {
	if roomNID, ok := d.Cache.GetRoomServerRoomNID(roomID); ok {
		return roomNID, nil
	}
	// Check if we already have a numeric ID in the database.
	roomNID, err := d.RoomsTable.SelectRoomNID(ctx, txn, roomID)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		roomNID, err = d.RoomsTable.InsertRoomNID(ctx, txn, roomID, roomVersion)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			roomNID, err = d.RoomsTable.SelectRoomNID(ctx, txn, roomID)
		}
	}
	if err == nil {
		d.Cache.StoreRoomServerRoomNID(roomID, roomNID)
	}
	return roomNID, err
}

func (d *Database) assignEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
	if eventTypeNID, ok := d.Cache.GetRoomServerEventTypeNID(eventType); ok {
		return eventTypeNID, nil
	}
	// Check if we already have a numeric ID in the database.
	eventTypeNID, err := d.EventTypesTable.SelectEventTypeNID(ctx, txn, eventType)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventTypeNID, err = d.EventTypesTable.InsertEventTypeNID(ctx, txn, eventType)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventTypeNID, err = d.EventTypesTable.SelectEventTypeNID(ctx, txn, eventType)
		}
	}
	if err == nil {
		d.Cache.StoreRoomServerEventTypeNID(eventType, eventTypeNID)
	}
	return eventTypeNID, err
}

func (d *Database) assignStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	if eventStateKeyNID, ok := d.Cache.GetRoomServerStateKeyNID(eventStateKey); ok {
		return eventStateKeyNID, nil
	}
	// Check if we already have a numeric ID in the database.
	eventStateKeyNID, err := d.EventStateKeysTable.SelectEventStateKeyNID(ctx, txn, eventStateKey)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventStateKeyNID, err = d.EventStateKeysTable.InsertEventStateKeyNID(ctx, txn, eventStateKey)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventStateKeyNID, err = d.EventStateKeysTable.SelectEventStateKeyNID(ctx, txn, eventStateKey)
		}
	}
	if err == nil {
		d.Cache.StoreRoomServerStateKeyNID(eventStateKey, eventStateKeyNID)
	}
	return eventStateKeyNID, err
}

func extractRoomVersionFromCreateEvent(event gomatrixserverlib.Event) (
	gomatrixserverlib.RoomVersion, error,
) {
	var err error
	var roomVersion gomatrixserverlib.RoomVersion
	// Look for m.room.create events.
	if event.Type() != gomatrixserverlib.MRoomCreate {
		return gomatrixserverlib.RoomVersion(""), nil
	}
	roomVersion = gomatrixserverlib.RoomVersionV1
	var createContent gomatrixserverlib.CreateContent
	// The m.room.create event contains an optional "room_version" key in
	// the event content, so we need to unmarshal that first.
	if err = json.Unmarshal(event.Content(), &createContent); err != nil {
		return gomatrixserverlib.RoomVersion(""), err
	}
	// A room version was specified in the event content?
	if createContent.RoomVersion != nil {
		roomVersion = gomatrixserverlib.RoomVersion(*createContent.RoomVersion)
	}
	return roomVersion, err
}

// handleRedactions manages the redacted status of events. There's two cases to consider in order to comply with the spec:
// "servers should not apply or send redactions to clients until both the redaction event and original event have been seen, and are valid."
// https://matrix.org/docs/spec/rooms/v3#authorization-rules-for-events
// These cases are:
//  - This is a redaction event, redact the event it references if we know about it.
//  - This is a normal event which may have been previously redacted.
// In the first case, check if we have the referenced event then apply the redaction, else store it
// in the redactions table with validated=FALSE. In the second case, check if there is a redaction for it:
// if there is then apply the redactions and set validated=TRUE.
//
// When an event is redacted, the redacted event JSON is modified to add an `unsigned.redacted_because` field. We use this field
// when loading events to determine whether to apply redactions. This keeps the hot-path of reading events quick as we don't need
// to cross-reference with other tables when loading.
//
// Returns the redaction event and the event ID of the redacted event if this call resulted in a redaction.
func (d *Database) handleRedactions(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, event gomatrixserverlib.Event,
) (*gomatrixserverlib.Event, string, error) {
	var err error
	isRedactionEvent := event.Type() == gomatrixserverlib.MRoomRedaction && event.StateKey() == nil
	if isRedactionEvent {
		// an event which redacts itself should be ignored
		if event.EventID() == event.Redacts() {
			return nil, "", nil
		}

		err = d.RedactionsTable.InsertRedaction(ctx, txn, tables.RedactionInfo{
			Validated:        false,
			RedactionEventID: event.EventID(),
			RedactsEventID:   event.Redacts(),
		})
		if err != nil {
			return nil, "", err
		}
	}

	redactionEvent, redactedEvent, validated, err := d.loadRedactionPair(ctx, txn, eventNID, event)
	if err != nil {
		return nil, "", err
	}
	if validated || redactedEvent == nil || redactionEvent == nil {
		// we've seen this redaction before or there is nothing to redact
		return nil, "", nil
	}
	if redactedEvent.RoomID() != redactionEvent.RoomID() {
		// redactions across rooms aren't allowed
		return nil, "", nil
	}

	// mark the event as redacted
	err = redactedEvent.SetUnsignedField("redacted_because", redactionEvent)
	if err != nil {
		return nil, "", err
	}
	if redactionsArePermanent {
		redactedEvent.Event = redactedEvent.Redact()
	}
	// overwrite the eventJSON table
	err = d.EventJSONTable.InsertEventJSON(ctx, txn, redactedEvent.EventNID, redactedEvent.JSON())
	if err != nil {
		return nil, "", err
	}

	return &redactionEvent.Event, redactedEvent.EventID(), d.RedactionsTable.MarkRedactionValidated(ctx, txn, redactionEvent.EventID(), true)
}

// loadRedactionPair returns both the redaction event and the redacted event, else nil.
func (d *Database) loadRedactionPair(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, event gomatrixserverlib.Event,
) (*types.Event, *types.Event, bool, error) {
	var redactionEvent, redactedEvent *types.Event
	var info *tables.RedactionInfo
	var err error
	isRedactionEvent := event.Type() == gomatrixserverlib.MRoomRedaction && event.StateKey() == nil

	var eventBeingRedacted string
	if isRedactionEvent {
		eventBeingRedacted = event.Redacts()
		redactionEvent = &types.Event{
			EventNID: eventNID,
			Event:    event,
		}
	} else {
		eventBeingRedacted = event.EventID() // maybe, we'll see if we have info
		redactedEvent = &types.Event{
			EventNID: eventNID,
			Event:    event,
		}
	}

	info, err = d.RedactionsTable.SelectRedactionInfoByEventBeingRedacted(ctx, txn, eventBeingRedacted)
	if err != nil {
		return nil, nil, false, err
	}
	if info == nil {
		// this event hasn't been redacted or we don't have the redaction for it yet
		return nil, nil, false, nil
	}

	if isRedactionEvent {
		redactedEvent = d.loadEvent(ctx, info.RedactsEventID)
	} else {
		redactionEvent = d.loadEvent(ctx, info.RedactionEventID)
	}

	return redactionEvent, redactedEvent, info.Validated, nil
}

// applyRedactions will redact events that have an `unsigned.redacted_because` field.
func (d *Database) applyRedactions(events []types.Event) {
	for i := range events {
		if result := gjson.GetBytes(events[i].Unsigned(), "redacted_because"); result.Exists() {
			events[i].Event = events[i].Redact()
		}
	}
}

// loadEvent loads a single event or returns nil on any problems/missing event
func (d *Database) loadEvent(ctx context.Context, eventID string) *types.Event {
	nids, err := d.EventNIDs(ctx, []string{eventID})
	if err != nil {
		return nil
	}
	if len(nids) == 0 {
		return nil
	}
	evs, err := d.Events(ctx, []types.EventNID{nids[eventID]})
	if err != nil {
		return nil
	}
	if len(evs) != 1 {
		return nil
	}
	return &evs[0]
}

// GetStateEvent returns the current state event of a given type for a given room with a given state key
// If no event could be found, returns nil
// If there was an issue during the retrieval, returns an error
func (d *Database) GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error) {
	roomInfo, err := d.RoomInfo(ctx, roomID)
	if err != nil {
		return nil, err
	}
	eventTypeNID, err := d.EventTypesTable.SelectEventTypeNID(ctx, nil, evType)
	if err != nil {
		return nil, err
	}
	stateKeyNID, err := d.EventStateKeysTable.SelectEventStateKeyNID(ctx, nil, stateKey)
	if err != nil {
		return nil, err
	}
	entries, err := d.loadStateAtSnapshot(ctx, roomInfo.StateSnapshotNID)
	if err != nil {
		return nil, err
	}
	// return the event requested
	for _, e := range entries {
		if e.EventTypeNID == eventTypeNID && e.EventStateKeyNID == stateKeyNID {
			data, err := d.EventJSONTable.BulkSelectEventJSON(ctx, []types.EventNID{e.EventNID})
			if err != nil {
				return nil, err
			}
			if len(data) == 0 {
				return nil, fmt.Errorf("GetStateEvent: no json for event nid %d", e.EventNID)
			}
			ev, err := gomatrixserverlib.NewEventFromTrustedJSON(data[0].EventJSON, false, roomInfo.RoomVersion)
			if err != nil {
				return nil, err
			}
			h := ev.Headered(roomInfo.RoomVersion)
			return &h, nil
		}
	}

	return nil, fmt.Errorf("GetStateEvent: no event type '%s' with key '%s' exists in room %s", evType, stateKey, roomID)
}

// GetRoomsByMembership returns a list of room IDs matching the provided membership and user ID (as state_key).
func (d *Database) GetRoomsByMembership(ctx context.Context, userID, membership string) ([]string, error) {
	var membershipState tables.MembershipState
	switch membership {
	case "join":
		membershipState = tables.MembershipStateJoin
	case "invite":
		membershipState = tables.MembershipStateInvite
	case "leave":
		membershipState = tables.MembershipStateLeaveOrBan
	case "ban":
		membershipState = tables.MembershipStateLeaveOrBan
	default:
		return nil, fmt.Errorf("GetRoomsByMembership: invalid membership %s", membership)
	}
	stateKeyNID, err := d.EventStateKeysTable.SelectEventStateKeyNID(ctx, nil, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("GetRoomsByMembership: cannot map user ID to state key NID: %w", err)
	}
	roomNIDs, err := d.MembershipTable.SelectRoomsWithMembership(ctx, stateKeyNID, membershipState)
	if err != nil {
		return nil, fmt.Errorf("GetRoomsByMembership: failed to SelectRoomsWithMembership: %w", err)
	}
	roomIDs, err := d.RoomsTable.BulkSelectRoomIDs(ctx, roomNIDs)
	if err != nil {
		return nil, fmt.Errorf("GetRoomsByMembership: failed to lookup room nids: %w", err)
	}
	if len(roomIDs) != len(roomNIDs) {
		return nil, fmt.Errorf("GetRoomsByMembership: missing room IDs, got %d want %d", len(roomIDs), len(roomNIDs))
	}
	return roomIDs, nil
}

// GetBulkStateContent returns all state events which match a given room ID and a given state key tuple. Both must be satisfied for a match.
// If a tuple has the StateKey of '*' and allowWildcards=true then all state events with the EventType should be returned.
func (d *Database) GetBulkStateContent(ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool) ([]csstables.StrippedEvent, error) {
	return nil, fmt.Errorf("not implemented yet")
}

// JoinedUsersSetInRooms returns all joined users in the rooms given, along with the count of how many times they appear.
func (d *Database) JoinedUsersSetInRooms(ctx context.Context, roomIDs []string) (map[string]int, error) {
	roomNIDs, err := d.RoomsTable.BulkSelectRoomNIDs(ctx, roomIDs)
	if err != nil {
		return nil, err
	}
	userNIDToCount, err := d.MembershipTable.SelectJoinedUsersSetForRooms(ctx, roomNIDs)
	if err != nil {
		return nil, err
	}
	stateKeyNIDs := make([]types.EventStateKeyNID, len(userNIDToCount))
	i := 0
	for nid := range userNIDToCount {
		stateKeyNIDs[i] = nid
		i++
	}
	nidToUserID, err := d.EventStateKeysTable.BulkSelectEventStateKey(ctx, stateKeyNIDs)
	if err != nil {
		return nil, err
	}
	if len(nidToUserID) != len(userNIDToCount) {
		return nil, fmt.Errorf("found %d users but only have state key nids for %d of them", len(userNIDToCount), len(nidToUserID))
	}
	result := make(map[string]int, len(userNIDToCount))
	for nid, count := range userNIDToCount {
		result[nidToUserID[nid]] = count
	}
	return result, nil
}

// GetKnownUsers searches all users that userID knows about.
func (d *Database) GetKnownUsers(ctx context.Context, userID, searchString string, limit int) ([]string, error) {
	stateKeyNID, err := d.EventStateKeysTable.SelectEventStateKeyNID(ctx, nil, userID)
	if err != nil {
		return nil, err
	}
	return d.MembershipTable.SelectKnownUsers(ctx, stateKeyNID, searchString, limit)
}

// GetKnownRooms returns a list of all rooms we know about.
func (d *Database) GetKnownRooms(ctx context.Context) ([]string, error) {
	return d.RoomsTable.SelectRoomIDs(ctx)
}

// FIXME TODO: Remove all this - horrible dupe with roomserver/state. Can't use the original impl because of circular loops
// it should live in this package!

func (d *Database) loadStateAtSnapshot(
	ctx context.Context, stateNID types.StateSnapshotNID,
) ([]types.StateEntry, error) {
	stateBlockNIDLists, err := d.StateBlockNIDs(ctx, []types.StateSnapshotNID{stateNID})
	if err != nil {
		return nil, err
	}
	// We've asked for exactly one snapshot from the db so we should have exactly one entry in the result.
	stateBlockNIDList := stateBlockNIDLists[0]

	stateEntryLists, err := d.StateEntries(ctx, stateBlockNIDList.StateBlockNIDs)
	if err != nil {
		return nil, err
	}
	stateEntriesMap := stateEntryListMap(stateEntryLists)

	// Combine all the state entries for this snapshot.
	// The order of state block NIDs in the list tells us the order to combine them in.
	var fullState []types.StateEntry
	for _, stateBlockNID := range stateBlockNIDList.StateBlockNIDs {
		entries, ok := stateEntriesMap.lookup(stateBlockNID)
		if !ok {
			// This should only get hit if the database is corrupt.
			// It should be impossible for an event to reference a NID that doesn't exist
			panic(fmt.Errorf("Corrupt DB: Missing state block numeric ID %d", stateBlockNID))
		}
		fullState = append(fullState, entries...)
	}

	// Stable sort so that the most recent entry for each state key stays
	// remains later in the list than the older entries for the same state key.
	sort.Stable(stateEntryByStateKeySorter(fullState))
	// Unique returns the last entry and hence the most recent entry for each state key.
	fullState = fullState[:util.Unique(stateEntryByStateKeySorter(fullState))]
	return fullState, nil
}

type stateEntryListMap []types.StateEntryList

func (m stateEntryListMap) lookup(stateBlockNID types.StateBlockNID) (stateEntries []types.StateEntry, ok bool) {
	list := []types.StateEntryList(m)
	i := sort.Search(len(list), func(i int) bool {
		return list[i].StateBlockNID >= stateBlockNID
	})
	if i < len(list) && list[i].StateBlockNID == stateBlockNID {
		ok = true
		stateEntries = list[i].StateEntries
	}
	return
}

type stateEntryByStateKeySorter []types.StateEntry

func (s stateEntryByStateKeySorter) Len() int { return len(s) }
func (s stateEntryByStateKeySorter) Less(i, j int) bool {
	return s[i].StateKeyTuple.LessThan(s[j].StateKeyTuple)
}
func (s stateEntryByStateKeySorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
