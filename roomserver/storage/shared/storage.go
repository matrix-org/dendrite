package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/matrix-org/dendrite/roomserver/version"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/tidwall/gjson"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
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
	DB *sql.DB
	EventDatabase
	Cache              caching.RoomServerCaches
	Writer             sqlutil.Writer
	RoomsTable         tables.Rooms
	StateSnapshotTable tables.StateSnapshot
	StateBlockTable    tables.StateBlock
	RoomAliasesTable   tables.RoomAliases
	InvitesTable       tables.Invites
	MembershipTable    tables.Membership
	PublishedTable     tables.Published
	Purge              tables.Purge
	GetRoomUpdaterFn   func(ctx context.Context, roomInfo *types.RoomInfo) (*RoomUpdater, error)
}

// EventDatabase contains all tables needed to work with events
type EventDatabase struct {
	DB                  *sql.DB
	Cache               caching.RoomServerCaches
	Writer              sqlutil.Writer
	EventsTable         tables.Events
	EventJSONTable      tables.EventJSON
	EventTypesTable     tables.EventTypes
	EventStateKeysTable tables.EventStateKeys
	PrevEventsTable     tables.PreviousEvents
	RedactionsTable     tables.Redactions
}

// GetEventDatabase returns an EventDatabase to work with events only.
func (d *Database) GetEventDatabase() *EventDatabase {
	db := &EventDatabase{
		DB:                  d.DB,
		Cache:               d.Cache,
		Writer:              d.Writer,
		EventsTable:         d.EventsTable,
		EventJSONTable:      d.EventJSONTable,
		EventTypesTable:     d.EventTypesTable,
		EventStateKeysTable: d.EventStateKeysTable,
		PrevEventsTable:     d.PrevEventsTable,
		RedactionsTable:     d.RedactionsTable,
	}
	return db
}

func (d *Database) SupportsConcurrentRoomInputs() bool {
	return true
}

func (d *Database) GetMembershipForHistoryVisibility(
	ctx context.Context, userNID types.EventStateKeyNID, roomInfo *types.RoomInfo, eventIDs ...string,
) (map[string]*gomatrixserverlib.HeaderedEvent, error) {
	return d.StateSnapshotTable.BulkSelectMembershipForHistoryVisibility(ctx, nil, userNID, roomInfo, eventIDs...)
}

func (d *EventDatabase) EventTypeNIDs(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	return d.eventTypeNIDs(ctx, nil, eventTypes)
}

func (d *EventDatabase) eventTypeNIDs(
	ctx context.Context, txn *sql.Tx, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	result := make(map[string]types.EventTypeNID)
	// first try the cache
	fetchEventTypes := make([]string, 0, len(eventTypes))
	for _, eventType := range eventTypes {
		eventTypeNID, ok := d.Cache.GetEventTypeKey(eventType)
		if ok {
			result[eventType] = eventTypeNID
			continue
		}
		fetchEventTypes = append(fetchEventTypes, eventType)
	}
	if len(fetchEventTypes) > 0 {
		nids, err := d.EventTypesTable.BulkSelectEventTypeNID(ctx, txn, fetchEventTypes)
		if err != nil {
			return nil, err
		}
		for eventType, nid := range nids {
			result[eventType] = nid
			d.Cache.StoreEventTypeKey(nid, eventType)
		}
	}
	return result, nil
}

func (d *EventDatabase) EventStateKeys(
	ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	result := make(map[types.EventStateKeyNID]string, len(eventStateKeyNIDs))
	fetch := make([]types.EventStateKeyNID, 0, len(eventStateKeyNIDs))
	for _, nid := range eventStateKeyNIDs {
		if key, ok := d.Cache.GetEventStateKey(nid); ok {
			result[nid] = key
		} else {
			fetch = append(fetch, nid)
		}
	}
	if len(fetch) > 0 {
		fromDB, err := d.EventStateKeysTable.BulkSelectEventStateKey(ctx, nil, fetch)
		if err != nil {
			return nil, err
		}
		for nid, key := range fromDB {
			result[nid] = key
			d.Cache.StoreEventStateKey(nid, key)
		}
	}
	return result, nil
}

func (d *EventDatabase) EventStateKeyNIDs(
	ctx context.Context, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	return d.eventStateKeyNIDs(ctx, nil, eventStateKeys)
}

func (d *EventDatabase) eventStateKeyNIDs(
	ctx context.Context, txn *sql.Tx, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	result := make(map[string]types.EventStateKeyNID)
	eventStateKeys = util.UniqueStrings(eventStateKeys)
	// first ask the cache about these keys
	fetchEventStateKeys := make([]string, 0, len(eventStateKeys))
	for _, eventStateKey := range eventStateKeys {
		eventStateKeyNID, ok := d.Cache.GetEventStateKeyNID(eventStateKey)
		if ok {
			result[eventStateKey] = eventStateKeyNID
			continue
		}
		fetchEventStateKeys = append(fetchEventStateKeys, eventStateKey)
	}

	if len(fetchEventStateKeys) > 0 {
		nids, err := d.EventStateKeysTable.BulkSelectEventStateKeyNID(ctx, txn, fetchEventStateKeys)
		if err != nil {
			return nil, err
		}
		for eventStateKey, nid := range nids {
			result[eventStateKey] = nid
			d.Cache.StoreEventStateKey(nid, eventStateKey)
		}
	}

	// We received some nids, but are still missing some, work out which and create them
	if len(eventStateKeys) > len(result) {
		var nid types.EventStateKeyNID
		var err error
		err = d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
			for _, eventStateKey := range eventStateKeys {
				if _, ok := result[eventStateKey]; ok {
					continue
				}

				nid, err = d.assignStateKeyNID(ctx, txn, eventStateKey)
				if err != nil {
					return err
				}
				result[eventStateKey] = nid
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (d *EventDatabase) StateEntriesForEventIDs(
	ctx context.Context, eventIDs []string, excludeRejected bool,
) ([]types.StateEntry, error) {
	return d.EventsTable.BulkSelectStateEventByID(ctx, nil, eventIDs, excludeRejected)
}

func (d *Database) StateEntriesForTuples(
	ctx context.Context,
	stateBlockNIDs []types.StateBlockNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	return d.stateEntriesForTuples(ctx, nil, stateBlockNIDs, stateKeyTuples)
}

func (d *Database) stateEntriesForTuples(
	ctx context.Context, txn *sql.Tx,
	stateBlockNIDs []types.StateBlockNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	entries, err := d.StateBlockTable.BulkSelectStateBlockEntries(
		ctx, txn, stateBlockNIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("d.StateBlockTable.BulkSelectStateBlockEntries: %w", err)
	}
	lists := []types.StateEntryList{}
	for i, entry := range entries {
		entries, err := d.EventsTable.BulkSelectStateEventByNID(ctx, txn, entry, stateKeyTuples)
		if err != nil {
			return nil, fmt.Errorf("d.EventsTable.BulkSelectStateEventByNID: %w", err)
		}
		lists = append(lists, types.StateEntryList{
			StateBlockNID: stateBlockNIDs[i],
			StateEntries:  entries,
		})
	}
	return lists, nil
}

func (d *Database) RoomInfoByNID(ctx context.Context, roomNID types.RoomNID) (*types.RoomInfo, error) {
	roomIDs, err := d.RoomsTable.BulkSelectRoomIDs(ctx, nil, []types.RoomNID{roomNID})
	if err != nil {
		return nil, err
	}
	if len(roomIDs) == 0 {
		return nil, fmt.Errorf("room does not exist")
	}
	return d.roomInfo(ctx, nil, roomIDs[0])
}

func (d *Database) RoomInfo(ctx context.Context, roomID string) (*types.RoomInfo, error) {
	return d.roomInfo(ctx, nil, roomID)
}

func (d *Database) roomInfo(ctx context.Context, txn *sql.Tx, roomID string) (*types.RoomInfo, error) {
	roomInfo, err := d.RoomsTable.SelectRoomInfo(ctx, txn, roomID)
	if err != nil {
		return nil, err
	}
	if roomInfo != nil {
		d.Cache.StoreRoomServerRoomID(roomInfo.RoomNID, roomID)
		d.Cache.StoreRoomVersion(roomID, roomInfo.RoomVersion)
	}
	return roomInfo, err
}

func (d *Database) AddState(
	ctx context.Context,
	roomNID types.RoomNID,
	stateBlockNIDs []types.StateBlockNID,
	state []types.StateEntry,
) (stateNID types.StateSnapshotNID, err error) {
	return d.addState(ctx, nil, roomNID, stateBlockNIDs, state)
}

func (d *Database) addState(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID,
	stateBlockNIDs []types.StateBlockNID,
	state []types.StateEntry,
) (stateNID types.StateSnapshotNID, err error) {
	if len(stateBlockNIDs) > 0 && len(state) > 0 {
		// Check to see if the event already appears in any of the existing state
		// blocks. If it does then we should not add it again, as this will just
		// result in excess state blocks and snapshots.
		// TODO: Investigate why this is happening - probably input_events.go!
		blocks, berr := d.StateBlockTable.BulkSelectStateBlockEntries(ctx, txn, stateBlockNIDs)
		if berr != nil {
			return 0, fmt.Errorf("d.StateBlockTable.BulkSelectStateBlockEntries: %w", berr)
		}
		var found bool
		for i := len(state) - 1; i >= 0; i-- {
			found = false
			for _, events := range blocks {
				for _, event := range events {
					if state[i].EventNID == event {
						found = true
						break
					}
				}
			}
			if found {
				state = append(state[:i], state[i+1:]...)
				i--
			}
		}
	}
	err = d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
		if len(state) > 0 {
			// If there's any state left to add then let's add new blocks.
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

func (d *EventDatabase) EventNIDs(
	ctx context.Context, eventIDs []string,
) (map[string]types.EventMetadata, error) {
	return d.eventNIDs(ctx, nil, eventIDs, NoFilter)
}

type UnsentFilter bool

const (
	NoFilter         UnsentFilter = false
	FilterUnsentOnly UnsentFilter = true
)

func (d *EventDatabase) eventNIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string, filter UnsentFilter,
) (map[string]types.EventMetadata, error) {
	switch filter {
	case FilterUnsentOnly:
		return d.EventsTable.BulkSelectUnsentEventNID(ctx, txn, eventIDs)
	case NoFilter:
		return d.EventsTable.BulkSelectEventNID(ctx, txn, eventIDs)
	default:
		panic("impossible case")
	}
}

func (d *EventDatabase) SetState(
	ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.EventsTable.UpdateEventState(ctx, txn, eventNID, stateNID)
	})
}

func (d *EventDatabase) StateAtEventIDs(
	ctx context.Context, eventIDs []string,
) ([]types.StateAtEvent, error) {
	return d.EventsTable.BulkSelectStateAtEventByID(ctx, nil, eventIDs)
}

func (d *EventDatabase) SnapshotNIDFromEventID(
	ctx context.Context, eventID string,
) (types.StateSnapshotNID, error) {
	return d.snapshotNIDFromEventID(ctx, nil, eventID)
}

func (d *EventDatabase) snapshotNIDFromEventID(
	ctx context.Context, txn *sql.Tx, eventID string,
) (types.StateSnapshotNID, error) {
	_, stateNID, err := d.EventsTable.SelectEvent(ctx, txn, eventID)
	if err != nil {
		return 0, err
	}
	if stateNID == 0 {
		return 0, sql.ErrNoRows // effectively there's no state entry
	}
	return stateNID, err
}

func (d *EventDatabase) EventIDs(
	ctx context.Context, eventNIDs []types.EventNID,
) (map[types.EventNID]string, error) {
	return d.EventsTable.BulkSelectEventID(ctx, nil, eventNIDs)
}

func (d *EventDatabase) EventsFromIDs(ctx context.Context, roomInfo *types.RoomInfo, eventIDs []string) ([]types.Event, error) {
	return d.eventsFromIDs(ctx, nil, roomInfo, eventIDs, NoFilter)
}

func (d *EventDatabase) eventsFromIDs(ctx context.Context, txn *sql.Tx, roomInfo *types.RoomInfo, eventIDs []string, filter UnsentFilter) ([]types.Event, error) {
	nidMap, err := d.eventNIDs(ctx, txn, eventIDs, filter)
	if err != nil {
		return nil, err
	}

	var nids []types.EventNID
	for _, nid := range nidMap {
		nids = append(nids, nid.EventNID)
	}

	return d.events(ctx, txn, roomInfo, nids)
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
	return d.stateBlockNIDs(ctx, nil, stateNIDs)
}

func (d *Database) stateBlockNIDs(
	ctx context.Context, txn *sql.Tx, stateNIDs []types.StateSnapshotNID,
) ([]types.StateBlockNIDList, error) {
	return d.StateSnapshotTable.BulkSelectStateBlockNIDs(ctx, txn, stateNIDs)
}

func (d *Database) StateEntries(
	ctx context.Context, stateBlockNIDs []types.StateBlockNID,
) ([]types.StateEntryList, error) {
	return d.stateEntries(ctx, nil, stateBlockNIDs)
}

func (d *Database) stateEntries(
	ctx context.Context, txn *sql.Tx, stateBlockNIDs []types.StateBlockNID,
) ([]types.StateEntryList, error) {
	entries, err := d.StateBlockTable.BulkSelectStateBlockEntries(
		ctx, txn, stateBlockNIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("d.StateBlockTable.BulkSelectStateBlockEntries: %w", err)
	}
	lists := make([]types.StateEntryList, 0, len(entries))
	for i, entry := range entries {
		eventNIDs, err := d.EventsTable.BulkSelectStateEventByNID(ctx, txn, entry, nil)
		if err != nil {
			return nil, fmt.Errorf("d.EventsTable.BulkSelectStateEventByNID: %w", err)
		}
		lists = append(lists, types.StateEntryList{
			StateBlockNID: stateBlockNIDs[i],
			StateEntries:  eventNIDs,
		})
	}
	return lists, nil
}

func (d *Database) SetRoomAlias(ctx context.Context, alias string, roomID string, creatorUserID string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.RoomAliasesTable.InsertRoomAlias(ctx, txn, alias, roomID, creatorUserID)
	})
}

func (d *Database) GetRoomIDForAlias(ctx context.Context, alias string) (string, error) {
	return d.RoomAliasesTable.SelectRoomIDFromAlias(ctx, nil, alias)
}

func (d *Database) GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error) {
	return d.RoomAliasesTable.SelectAliasesFromRoomID(ctx, nil, roomID)
}

func (d *Database) GetCreatorIDForAlias(
	ctx context.Context, alias string,
) (string, error) {
	return d.RoomAliasesTable.SelectCreatorIDFromAlias(ctx, nil, alias)
}

func (d *Database) RemoveRoomAlias(ctx context.Context, alias string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.RoomAliasesTable.DeleteRoomAlias(ctx, txn, alias)
	})
}

func (d *Database) GetMembership(ctx context.Context, roomNID types.RoomNID, requestSenderUserID string) (membershipEventNID types.EventNID, stillInRoom, isRoomforgotten bool, err error) {
	var requestSenderUserNID types.EventStateKeyNID
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		requestSenderUserNID, err = d.assignStateKeyNID(ctx, txn, requestSenderUserID)
		return err
	})
	if err != nil {
		return 0, false, false, fmt.Errorf("d.assignStateKeyNID: %w", err)
	}

	senderMembershipEventNID, senderMembership, isRoomforgotten, err :=
		d.MembershipTable.SelectMembershipFromRoomAndTarget(
			ctx, nil, roomNID, requestSenderUserNID,
		)
	if err == sql.ErrNoRows {
		// The user has never been a member of that room
		return 0, false, false, nil
	} else if err != nil {
		return
	}

	return senderMembershipEventNID, senderMembership == tables.MembershipStateJoin, isRoomforgotten, nil
}

func (d *Database) GetMembershipEventNIDsForRoom(
	ctx context.Context, roomNID types.RoomNID, joinOnly bool, localOnly bool,
) ([]types.EventNID, error) {
	return d.getMembershipEventNIDsForRoom(ctx, nil, roomNID, joinOnly, localOnly)
}

func (d *Database) getMembershipEventNIDsForRoom(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, joinOnly bool, localOnly bool,
) ([]types.EventNID, error) {
	if joinOnly {
		return d.MembershipTable.SelectMembershipsFromRoomAndMembership(
			ctx, txn, roomNID, tables.MembershipStateJoin, localOnly,
		)
	}

	return d.MembershipTable.SelectMembershipsFromRoom(ctx, txn, roomNID, localOnly)
}

func (d *Database) GetInvitesForUser(
	ctx context.Context,
	roomNID types.RoomNID,
	targetUserNID types.EventStateKeyNID,
) (senderUserIDs []types.EventStateKeyNID, eventIDs []string, inviteEventJSON []byte, err error) {
	return d.InvitesTable.SelectInviteActiveForUserInRoom(ctx, nil, targetUserNID, roomNID)
}

func (d *EventDatabase) Events(ctx context.Context, roomInfo *types.RoomInfo, eventNIDs []types.EventNID) ([]types.Event, error) {
	return d.events(ctx, nil, roomInfo, eventNIDs)
}

func (d *EventDatabase) events(
	ctx context.Context, txn *sql.Tx, roomInfo *types.RoomInfo, inputEventNIDs types.EventNIDs,
) ([]types.Event, error) {
	if roomInfo == nil {

		roomInfo = &types.RoomInfo{RoomVersion: version.DefaultRoomVersion()}
	}

	sort.Sort(inputEventNIDs)
	events := make(map[types.EventNID]*gomatrixserverlib.Event, len(inputEventNIDs))
	eventNIDs := make([]types.EventNID, 0, len(inputEventNIDs))
	for _, nid := range inputEventNIDs {
		if event, ok := d.Cache.GetRoomServerEvent(nid); ok && event != nil {
			events[nid] = event
		} else {
			eventNIDs = append(eventNIDs, nid)
		}
	}
	// If we don't need to get any events from the database, short circuit now
	if len(eventNIDs) == 0 {
		results := make([]types.Event, 0, len(inputEventNIDs))
		for _, nid := range inputEventNIDs {
			event, ok := events[nid]
			if !ok || event == nil {
				return nil, fmt.Errorf("event %d missing", nid)
			}
			results = append(results, types.Event{
				EventNID: nid,
				Event:    event,
			})
		}
		if !redactionsArePermanent {
			d.applyRedactions(results)
		}
	}
	eventJSONs, err := d.EventJSONTable.BulkSelectEventJSON(ctx, txn, eventNIDs)
	if err != nil {
		return nil, err
	}
	eventIDs, err := d.EventsTable.BulkSelectEventID(ctx, txn, eventNIDs)
	if err != nil {
		eventIDs = map[types.EventNID]string{}
	}

	for _, eventJSON := range eventJSONs {
		events[eventJSON.EventNID], err = gomatrixserverlib.NewEventFromTrustedJSONWithEventID(
			eventIDs[eventJSON.EventNID], eventJSON.EventJSON, false, roomInfo.RoomVersion,
		)
		if err != nil {
			return nil, err
		}
		if event := events[eventJSON.EventNID]; event != nil {
			d.Cache.StoreRoomServerEvent(eventJSON.EventNID, event)
		}
	}
	results := make([]types.Event, 0, len(inputEventNIDs))
	for _, nid := range inputEventNIDs {
		event, ok := events[nid]
		if !ok || event == nil {
			return nil, fmt.Errorf("event %d missing", nid)
		}
		results = append(results, types.Event{
			EventNID: nid,
			Event:    event,
		})
	}
	if !redactionsArePermanent {
		d.applyRedactions(results)
	}
	return results, nil
}

func (d *Database) BulkSelectSnapshotsFromEventIDs(
	ctx context.Context, eventIDs []string,
) (map[types.StateSnapshotNID][]string, error) {
	return d.EventsTable.BulkSelectSnapshotsFromEventIDs(ctx, nil, eventIDs)
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
		return err
	})
	return updater, err
}

func (d *Database) GetRoomUpdater(
	ctx context.Context, roomInfo *types.RoomInfo,
) (*RoomUpdater, error) {
	if d.GetRoomUpdaterFn != nil {
		return d.GetRoomUpdaterFn(ctx, roomInfo)
	}
	txn, err := d.DB.Begin()
	if err != nil {
		return nil, err
	}
	var updater *RoomUpdater
	_ = d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
		updater, err = NewRoomUpdater(ctx, d, txn, roomInfo)
		return err
	})
	return updater, err
}

func (d *Database) IsEventRejected(ctx context.Context, roomNID types.RoomNID, eventID string) (bool, error) {
	return d.EventsTable.SelectEventRejected(ctx, nil, roomNID, eventID)
}

// GetOrCreateRoomNID gets or creates a new roomNID for the given event. Also returns a RoomInfo, which is only safe to use
// with functions only needing a roomVersion or roomNID.
func (d *Database) GetOrCreateRoomNID(ctx context.Context, event *gomatrixserverlib.Event) (roomNID types.RoomNID, roomInfo *types.RoomInfo, err error) {
	// Get the default room version. If the client doesn't supply a room_version
	// then we will use our configured default to create the room.
	// https://matrix.org/docs/spec/client_server/r0.6.0#post-matrix-client-r0-createroom
	// Note that the below logic depends on the m.room.create event being the
	// first event that is persisted to the database when creating or joining a
	// room.
	var roomVersion gomatrixserverlib.RoomVersion
	if roomVersion, err = extractRoomVersionFromCreateEvent(event); err != nil {
		return 0, nil, fmt.Errorf("extractRoomVersionFromCreateEvent: %w", err)
	}
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		roomNID, err = d.assignRoomNID(ctx, txn, event.RoomID(), roomVersion)
		if err != nil {
			return err
		}
		return nil
	})
	return roomNID, &types.RoomInfo{
		RoomVersion: roomVersion,
		RoomNID:     roomNID,
	}, err
}

func (d *Database) GetOrCreateEventTypeNID(ctx context.Context, eventType string) (eventTypeNID types.EventTypeNID, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if eventTypeNID, err = d.assignEventTypeNID(ctx, txn, eventType); err != nil {
			return fmt.Errorf("d.assignEventTypeNID: %w", err)
		}
		return nil
	})
	return eventTypeNID, err
}

func (d *Database) GetOrCreateEventStateKeyNID(ctx context.Context, eventStateKey *string) (eventStateKeyNID types.EventStateKeyNID, err error) {
	if eventStateKey == nil {
		return 0, nil
	}

	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if eventStateKeyNID, err = d.assignStateKeyNID(ctx, txn, *eventStateKey); err != nil {
			return fmt.Errorf("d.assignStateKeyNID: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return eventStateKeyNID, nil
}

func (d *EventDatabase) StoreEvent(
	ctx context.Context, event *gomatrixserverlib.Event,
	roomInfo *types.RoomInfo, eventTypeNID types.EventTypeNID, eventStateKeyNID types.EventStateKeyNID,
	authEventNIDs []types.EventNID, isRejected bool,
) (types.EventNID, types.StateAtEvent, error) {
	var (
		eventNID types.EventNID
		stateNID types.StateSnapshotNID
		err      error
	)

	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if eventNID, stateNID, err = d.EventsTable.InsertEvent(
			ctx,
			txn,
			roomInfo.RoomNID,
			eventTypeNID,
			eventStateKeyNID,
			event.EventID(),
			event.EventReference().EventSHA256,
			authEventNIDs,
			event.Depth(),
			isRejected,
		); err != nil {
			if err == sql.ErrNoRows {
				// We've already inserted the event so select the numeric event ID
				eventNID, stateNID, err = d.EventsTable.SelectEvent(ctx, txn, event.EventID())
			} else if err != nil {
				return fmt.Errorf("d.EventsTable.InsertEvent: %w", err)
			}
			if err != nil {
				return fmt.Errorf("d.EventsTable.SelectEvent: %w", err)
			}
		}

		if err = d.EventJSONTable.InsertEventJSON(ctx, txn, eventNID, event.JSON()); err != nil {
			return fmt.Errorf("d.EventJSONTable.InsertEventJSON: %w", err)
		}

		if prevEvents := event.PrevEvents(); len(prevEvents) > 0 {
			// Create an updater - NB: on sqlite this WILL create a txn as we are directly calling the shared DB form of
			// GetLatestEventsForUpdate - not via the SQLiteDatabase form which has `nil` txns. This
			// function only does SELECTs though so the created txn (at this point) is just a read txn like
			// any other so this is fine. If we ever update GetLatestEventsForUpdate or NewLatestEventsUpdater
			// to do writes however then this will need to go inside `Writer.Do`.

			// The following is a copy of RoomUpdater.StorePreviousEvents
			for _, ref := range prevEvents {
				if err = d.PrevEventsTable.InsertPreviousEvent(ctx, txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
					return fmt.Errorf("u.d.PrevEventsTable.InsertPreviousEvent: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return 0, types.StateAtEvent{}, fmt.Errorf("d.Writer.Do: %w", err)
	}

	// We should attempt to update the previous events table with any
	// references that this new event makes. We do this using a latest
	// events updater because it somewhat works as a mutex, ensuring
	// that there's a row-level lock on the latest room events (well,
	// on Postgres at least).

	return eventNID, types.StateAtEvent{
		BeforeStateSnapshotNID: stateNID,
		StateEntry: types.StateEntry{
			StateKeyTuple: types.StateKeyTuple{
				EventTypeNID:     eventTypeNID,
				EventStateKeyNID: eventStateKeyNID,
			},
			EventNID: eventNID,
		},
	}, err
}

func (d *Database) PublishRoom(ctx context.Context, roomID, appserviceID, networkID string, publish bool) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.PublishedTable.UpsertRoomPublished(ctx, txn, roomID, appserviceID, networkID, publish)
	})
}

func (d *Database) GetPublishedRoom(ctx context.Context, roomID string) (bool, error) {
	return d.PublishedTable.SelectPublishedFromRoomID(ctx, nil, roomID)
}

func (d *Database) GetPublishedRooms(ctx context.Context, networkID string, includeAllNetworks bool) ([]string, error) {
	return d.PublishedTable.SelectAllPublishedRooms(ctx, nil, networkID, true, includeAllNetworks)
}

func (d *Database) MissingAuthPrevEvents(
	ctx context.Context, e *gomatrixserverlib.Event,
) (missingAuth, missingPrev []string, err error) {
	authEventNIDs, err := d.EventNIDs(ctx, e.AuthEventIDs())
	if err != nil {
		return nil, nil, fmt.Errorf("d.EventNIDs: %w", err)
	}
	for _, authEventID := range e.AuthEventIDs() {
		if _, ok := authEventNIDs[authEventID]; !ok {
			missingAuth = append(missingAuth, authEventID)
		}
	}

	for _, prevEventID := range e.PrevEventIDs() {
		state, err := d.StateAtEventIDs(ctx, []string{prevEventID})
		if err != nil || len(state) == 0 || (!state[0].IsCreate() && state[0].BeforeStateSnapshotNID == 0) {
			missingPrev = append(missingPrev, prevEventID)
		}
	}

	return
}

func (d *Database) assignRoomNID(
	ctx context.Context, txn *sql.Tx, roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (types.RoomNID, error) {
	roomNID, ok := d.Cache.GetRoomServerRoomNID(roomID)
	if ok {
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
	if err != nil {
		return 0, err
	}
	d.Cache.StoreRoomServerRoomID(roomNID, roomID)
	return roomNID, nil
}

func (d *Database) assignEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
	eventTypeNID, ok := d.Cache.GetEventTypeKey(eventType)
	if ok {
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
	if err != nil {
		return 0, err
	}
	d.Cache.StoreEventTypeKey(eventTypeNID, eventType)
	return eventTypeNID, nil
}

func (d *EventDatabase) assignStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	eventStateKeyNID, ok := d.Cache.GetEventStateKeyNID(eventStateKey)
	if ok {
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
	d.Cache.StoreEventStateKey(eventStateKeyNID, eventStateKey)
	return eventStateKeyNID, err
}

func extractRoomVersionFromCreateEvent(event *gomatrixserverlib.Event) (
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

// MaybeRedactEvent manages the redacted status of events. There's two cases to consider in order to comply with the spec:
// "servers should not apply or send redactions to clients until both the redaction event and original event have been seen, and are valid."
// https://matrix.org/docs/spec/rooms/v3#authorization-rules-for-events
// These cases are:
//   - This is a redaction event, redact the event it references if we know about it.
//   - This is a normal event which may have been previously redacted.
//
// In the first case, check if we have the referenced event then apply the redaction, else store it
// in the redactions table with validated=FALSE. In the second case, check if there is a redaction for it:
// if there is then apply the redactions and set validated=TRUE.
//
// When an event is redacted, the redacted event JSON is modified to add an `unsigned.redacted_because` field. We use this field
// when loading events to determine whether to apply redactions. This keeps the hot-path of reading events quick as we don't need
// to cross-reference with other tables when loading.
//
// Returns the redaction event and the redacted event if this call resulted in a redaction.
func (d *EventDatabase) MaybeRedactEvent(
	ctx context.Context, roomInfo *types.RoomInfo, eventNID types.EventNID, event *gomatrixserverlib.Event, redactAllowed bool,
) (*gomatrixserverlib.Event, *gomatrixserverlib.Event, error) {
	var (
		redactionEvent, redactedEvent *types.Event
		err                           error
		validated                     bool
		ignoreRedaction               bool
	)

	wErr := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		isRedactionEvent := event.Type() == gomatrixserverlib.MRoomRedaction && event.StateKey() == nil
		if isRedactionEvent {
			// an event which redacts itself should be ignored
			if event.EventID() == event.Redacts() {
				return nil
			}

			err = d.RedactionsTable.InsertRedaction(ctx, txn, tables.RedactionInfo{
				Validated:        false,
				RedactionEventID: event.EventID(),
				RedactsEventID:   event.Redacts(),
			})
			if err != nil {
				return fmt.Errorf("d.RedactionsTable.InsertRedaction: %w", err)
			}
		}

		redactionEvent, redactedEvent, validated, err = d.loadRedactionPair(ctx, txn, roomInfo, eventNID, event)
		switch {
		case err != nil:
			return fmt.Errorf("d.loadRedactionPair: %w", err)
		case validated || redactedEvent == nil || redactionEvent == nil:
			// we've seen this redaction before or there is nothing to redact
			return nil
		case redactedEvent.RoomID() != redactionEvent.RoomID():
			// redactions across rooms aren't allowed
			ignoreRedaction = true
			return nil
		}

		// 1. The power level of the redaction event’s sender is greater than or equal to the redact level. (redactAllowed)
		// 2. The domain of the redaction event’s sender matches that of the original event’s sender.
		_, sender1, _ := gomatrixserverlib.SplitID('@', redactedEvent.Sender())
		_, sender2, _ := gomatrixserverlib.SplitID('@', redactionEvent.Sender())
		if !redactAllowed || sender1 != sender2 {
			ignoreRedaction = true
			return nil
		}

		// mark the event as redacted
		if redactionsArePermanent {
			redactedEvent.Redact()
		}

		err = redactedEvent.SetUnsignedField("redacted_because", redactionEvent)
		if err != nil {
			return fmt.Errorf("redactedEvent.SetUnsignedField: %w", err)
		}
		// NOTSPEC: sytest relies on this unspecced field existing :(
		err = redactedEvent.SetUnsignedField("redacted_by", redactionEvent.EventID())
		if err != nil {
			return fmt.Errorf("redactedEvent.SetUnsignedField: %w", err)
		}
		// overwrite the eventJSON table
		err = d.EventJSONTable.InsertEventJSON(ctx, txn, redactedEvent.EventNID, redactedEvent.JSON())
		if err != nil {
			return fmt.Errorf("d.EventJSONTable.InsertEventJSON: %w", err)
		}

		err = d.RedactionsTable.MarkRedactionValidated(ctx, txn, redactionEvent.EventID(), true)
		if err != nil {
			return fmt.Errorf("d.RedactionsTable.MarkRedactionValidated: %w", err)
		}
		return nil
	})
	if wErr != nil {
		return nil, nil, err
	}
	if ignoreRedaction || redactionEvent == nil || redactedEvent == nil {
		return nil, nil, nil
	}
	return redactionEvent.Event, redactedEvent.Event, nil
}

// loadRedactionPair returns both the redaction event and the redacted event, else nil.
func (d *EventDatabase) loadRedactionPair(
	ctx context.Context, txn *sql.Tx, roomInfo *types.RoomInfo, eventNID types.EventNID, event *gomatrixserverlib.Event,
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
		redactedEvent = d.loadEvent(ctx, roomInfo, info.RedactsEventID)
	} else {
		redactionEvent = d.loadEvent(ctx, roomInfo, info.RedactionEventID)
	}

	return redactionEvent, redactedEvent, info.Validated, nil
}

// applyRedactions will redact events that have an `unsigned.redacted_because` field.
func (d *EventDatabase) applyRedactions(events []types.Event) {
	for i := range events {
		if result := gjson.GetBytes(events[i].Unsigned(), "redacted_because"); result.Exists() {
			events[i].Redact()
		}
	}
}

// loadEvent loads a single event or returns nil on any problems/missing event
func (d *EventDatabase) loadEvent(ctx context.Context, roomInfo *types.RoomInfo, eventID string) *types.Event {
	nids, err := d.EventNIDs(ctx, []string{eventID})
	if err != nil {
		return nil
	}
	if len(nids) == 0 {
		return nil
	}
	evs, err := d.Events(ctx, roomInfo, []types.EventNID{nids[eventID].EventNID})
	if err != nil {
		return nil
	}
	if len(evs) != 1 {
		return nil
	}
	return &evs[0]
}

func (d *Database) GetHistoryVisibilityState(ctx context.Context, roomInfo *types.RoomInfo, eventID string, domain string) ([]*gomatrixserverlib.Event, error) {
	eventStates, err := d.EventsTable.BulkSelectStateAtEventByID(ctx, nil, []string{eventID})
	if err != nil {
		return nil, err
	}
	stateSnapshotNID := eventStates[0].BeforeStateSnapshotNID
	if stateSnapshotNID == 0 {
		return nil, nil
	}
	eventNIDs, err := d.StateSnapshotTable.BulkSelectStateForHistoryVisibility(ctx, nil, stateSnapshotNID, domain)
	if err != nil {
		return nil, err
	}
	eventIDs, _ := d.EventsTable.BulkSelectEventID(ctx, nil, eventNIDs)
	if err != nil {
		eventIDs = map[types.EventNID]string{}
	}
	events := make([]*gomatrixserverlib.Event, 0, len(eventNIDs))
	for _, eventNID := range eventNIDs {
		data, err := d.EventJSONTable.BulkSelectEventJSON(ctx, nil, []types.EventNID{eventNID})
		if err != nil {
			return nil, err
		}
		ev, err := gomatrixserverlib.NewEventFromTrustedJSONWithEventID(eventIDs[eventNID], data[0].EventJSON, false, roomInfo.RoomVersion)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

// GetStateEvent returns the current state event of a given type for a given room with a given state key
// If no event could be found, returns nil
// If there was an issue during the retrieval, returns an error
func (d *Database) GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error) {
	roomInfo, err := d.roomInfo(ctx, nil, roomID)
	if err != nil {
		return nil, err
	}
	if roomInfo == nil {
		return nil, fmt.Errorf("room %s doesn't exist", roomID)
	}
	// e.g invited rooms
	if roomInfo.IsStub() {
		return nil, nil
	}
	eventTypeNID, err := d.EventTypesTable.SelectEventTypeNID(ctx, nil, evType)
	if err == sql.ErrNoRows {
		// No rooms have an event of this type, otherwise we'd have an event type NID
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	stateKeyNID, err := d.EventStateKeysTable.SelectEventStateKeyNID(ctx, nil, stateKey)
	if err == sql.ErrNoRows {
		// No rooms have a state event with this state key, otherwise we'd have an state key NID
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := d.loadStateAtSnapshot(ctx, roomInfo.StateSnapshotNID())
	if err != nil {
		return nil, err
	}
	var eventNIDs []types.EventNID
	for _, e := range entries {
		if e.EventTypeNID == eventTypeNID && e.EventStateKeyNID == stateKeyNID {
			eventNIDs = append(eventNIDs, e.EventNID)
		}
	}
	eventIDs, _ := d.EventsTable.BulkSelectEventID(ctx, nil, eventNIDs)
	if err != nil {
		eventIDs = map[types.EventNID]string{}
	}
	// return the event requested
	for _, e := range entries {
		if e.EventTypeNID == eventTypeNID && e.EventStateKeyNID == stateKeyNID {
			data, err := d.EventJSONTable.BulkSelectEventJSON(ctx, nil, []types.EventNID{e.EventNID})
			if err != nil {
				return nil, err
			}
			if len(data) == 0 {
				return nil, fmt.Errorf("GetStateEvent: no json for event nid %d", e.EventNID)
			}
			ev, err := gomatrixserverlib.NewEventFromTrustedJSONWithEventID(eventIDs[e.EventNID], data[0].EventJSON, false, roomInfo.RoomVersion)
			if err != nil {
				return nil, err
			}
			return ev.Headered(roomInfo.RoomVersion), nil
		}
	}

	return nil, nil
}

// Same as GetStateEvent but returns all matching state events with this event type. Returns no error
// if there are no events with this event type.
func (d *Database) GetStateEventsWithEventType(ctx context.Context, roomID, evType string) ([]*gomatrixserverlib.HeaderedEvent, error) {
	roomInfo, err := d.roomInfo(ctx, nil, roomID)
	if err != nil {
		return nil, err
	}
	if roomInfo == nil {
		return nil, fmt.Errorf("room %s doesn't exist", roomID)
	}
	// e.g invited rooms
	if roomInfo.IsStub() {
		return nil, nil
	}
	eventTypeNID, err := d.EventTypesTable.SelectEventTypeNID(ctx, nil, evType)
	if err == sql.ErrNoRows {
		// No rooms have an event of this type, otherwise we'd have an event type NID
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := d.loadStateAtSnapshot(ctx, roomInfo.StateSnapshotNID())
	if err != nil {
		return nil, err
	}
	var eventNIDs []types.EventNID
	for _, e := range entries {
		if e.EventTypeNID == eventTypeNID {
			eventNIDs = append(eventNIDs, e.EventNID)
		}
	}
	eventIDs, _ := d.EventsTable.BulkSelectEventID(ctx, nil, eventNIDs)
	if err != nil {
		eventIDs = map[types.EventNID]string{}
	}
	// return the events requested
	eventPairs, err := d.EventJSONTable.BulkSelectEventJSON(ctx, nil, eventNIDs)
	if err != nil {
		return nil, err
	}
	if len(eventPairs) == 0 {
		return nil, nil
	}
	var result []*gomatrixserverlib.HeaderedEvent
	for _, pair := range eventPairs {
		ev, err := gomatrixserverlib.NewEventFromTrustedJSONWithEventID(eventIDs[pair.EventNID], pair.EventJSON, false, roomInfo.RoomVersion)
		if err != nil {
			return nil, err
		}
		result = append(result, ev.Headered(roomInfo.RoomVersion))
	}

	return result, nil
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
	roomNIDs, err := d.MembershipTable.SelectRoomsWithMembership(ctx, nil, stateKeyNID, membershipState)
	if err != nil {
		return nil, fmt.Errorf("GetRoomsByMembership: failed to SelectRoomsWithMembership: %w", err)
	}
	roomIDs, err := d.RoomsTable.BulkSelectRoomIDs(ctx, nil, roomNIDs)
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
func (d *Database) GetBulkStateContent(ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool) ([]tables.StrippedEvent, error) {
	eventTypes := make([]string, 0, len(tuples))
	for _, tuple := range tuples {
		eventTypes = append(eventTypes, tuple.EventType)
	}
	// we don't bother failing the request if we get asked for event types we don't know about, as all that would result in is no matches which
	// isn't a failure.
	eventTypeNIDMap, err := d.EventTypesTable.BulkSelectEventTypeNID(ctx, nil, eventTypes)
	if err != nil {
		return nil, fmt.Errorf("GetBulkStateContent: failed to map event type nids: %w", err)
	}
	typeNIDSet := make(map[types.EventTypeNID]bool)
	for _, nid := range eventTypeNIDMap {
		typeNIDSet[nid] = true
	}

	allowWildcard := make(map[types.EventTypeNID]bool)
	eventStateKeys := make([]string, 0, len(tuples))
	for _, tuple := range tuples {
		if allowWildcards && tuple.StateKey == "*" {
			allowWildcard[eventTypeNIDMap[tuple.EventType]] = true
			continue
		}
		eventStateKeys = append(eventStateKeys, tuple.StateKey)

	}

	eventStateKeyNIDMap, err := d.eventStateKeyNIDs(ctx, nil, eventStateKeys)
	if err != nil {
		return nil, fmt.Errorf("GetBulkStateContent: failed to map state key nids: %w", err)
	}
	stateKeyNIDSet := make(map[types.EventStateKeyNID]bool)
	for _, nid := range eventStateKeyNIDMap {
		stateKeyNIDSet[nid] = true
	}

	var eventNIDs []types.EventNID
	eventNIDToVer := make(map[types.EventNID]gomatrixserverlib.RoomVersion)
	// TODO: This feels like this is going to be really slow...
	for _, roomID := range roomIDs {
		roomInfo, err2 := d.roomInfo(ctx, nil, roomID)
		if err2 != nil {
			return nil, fmt.Errorf("GetBulkStateContent: failed to load room info for room %s : %w", roomID, err2)
		}
		// for unknown rooms or rooms which we don't have the current state, skip them.
		if roomInfo == nil || roomInfo.IsStub() {
			continue
		}
		entries, err2 := d.loadStateAtSnapshot(ctx, roomInfo.StateSnapshotNID())
		if err2 != nil {
			return nil, fmt.Errorf("GetBulkStateContent: failed to load state for room %s : %w", roomID, err2)
		}
		for _, entry := range entries {
			if typeNIDSet[entry.EventTypeNID] {
				if allowWildcard[entry.EventTypeNID] || stateKeyNIDSet[entry.EventStateKeyNID] {
					eventNIDs = append(eventNIDs, entry.EventNID)
					eventNIDToVer[entry.EventNID] = roomInfo.RoomVersion
				}
			}
		}
	}
	eventIDs, _ := d.EventsTable.BulkSelectEventID(ctx, nil, eventNIDs)
	if err != nil {
		eventIDs = map[types.EventNID]string{}
	}
	events, err := d.EventJSONTable.BulkSelectEventJSON(ctx, nil, eventNIDs)
	if err != nil {
		return nil, fmt.Errorf("GetBulkStateContent: failed to load event JSON for event nids: %w", err)
	}
	result := make([]tables.StrippedEvent, len(events))
	for i := range events {
		roomVer := eventNIDToVer[events[i].EventNID]
		ev, err := gomatrixserverlib.NewEventFromTrustedJSONWithEventID(eventIDs[events[i].EventNID], events[i].EventJSON, false, roomVer)
		if err != nil {
			return nil, fmt.Errorf("GetBulkStateContent: failed to load event JSON for event NID %v : %w", events[i].EventNID, err)
		}
		result[i] = tables.StrippedEvent{
			EventType:    ev.Type(),
			RoomID:       ev.RoomID(),
			StateKey:     *ev.StateKey(),
			ContentValue: tables.ExtractContentValue(ev.Headered(roomVer)),
		}
	}

	return result, nil
}

// JoinedUsersSetInRooms returns a map of how many times the given users appear in the specified rooms.
func (d *Database) JoinedUsersSetInRooms(ctx context.Context, roomIDs, userIDs []string, localOnly bool) (map[string]int, error) {
	roomNIDs, err := d.RoomsTable.BulkSelectRoomNIDs(ctx, nil, roomIDs)
	if err != nil {
		return nil, err
	}
	userNIDsMap, err := d.eventStateKeyNIDs(ctx, nil, userIDs)
	if err != nil {
		return nil, err
	}
	userNIDs := make([]types.EventStateKeyNID, 0, len(userNIDsMap))
	nidToUserID := make(map[types.EventStateKeyNID]string, len(userNIDsMap))
	for id, nid := range userNIDsMap {
		userNIDs = append(userNIDs, nid)
		nidToUserID[nid] = id
	}
	userNIDToCount, err := d.MembershipTable.SelectJoinedUsersSetForRooms(ctx, nil, roomNIDs, userNIDs, localOnly)
	if err != nil {
		return nil, err
	}
	stateKeyNIDs := make([]types.EventStateKeyNID, len(userNIDToCount))
	i := 0
	for nid := range userNIDToCount {
		stateKeyNIDs[i] = nid
		i++
	}
	// If we didn't have any userIDs to look up, get the UserIDs for the returned userNIDToCount now
	if len(userIDs) == 0 {
		nidToUserID, err = d.EventStateKeys(ctx, stateKeyNIDs)
		if err != nil {
			return nil, err
		}
	}
	result := make(map[string]int, len(userNIDToCount))
	for nid, count := range userNIDToCount {
		result[nidToUserID[nid]] = count
	}
	return result, nil
}

// GetLeftUsers calculates users we (the server) don't share a room with anymore.
func (d *Database) GetLeftUsers(ctx context.Context, userIDs []string) ([]string, error) {
	// Get the userNID for all users with a stale device list
	stateKeyNIDMap, err := d.EventStateKeyNIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	userNIDs := make([]types.EventStateKeyNID, 0, len(stateKeyNIDMap))
	userNIDtoUserID := make(map[types.EventStateKeyNID]string, len(stateKeyNIDMap))
	// Create a map from userNID -> userID
	for userID, nid := range stateKeyNIDMap {
		userNIDs = append(userNIDs, nid)
		userNIDtoUserID[nid] = userID
	}

	// Get all users whose membership is still join, knock or invite.
	stillJoinedUsersNIDs, err := d.MembershipTable.SelectJoinedUsers(ctx, nil, userNIDs)
	if err != nil {
		return nil, err
	}

	// Remove joined users from the "user with stale devices" list, which contains left AND joined users
	for _, joinedUser := range stillJoinedUsersNIDs {
		delete(userNIDtoUserID, joinedUser)
	}

	// The users still in our userNIDtoUserID map are the users we don't share a room with anymore,
	// and the return value we are looking for.
	leftUsers := make([]string, 0, len(userNIDtoUserID))
	for _, userID := range userNIDtoUserID {
		leftUsers = append(leftUsers, userID)
	}

	return leftUsers, nil
}

// GetLocalServerInRoom returns true if we think we're in a given room or false otherwise.
func (d *Database) GetLocalServerInRoom(ctx context.Context, roomNID types.RoomNID) (bool, error) {
	return d.MembershipTable.SelectLocalServerInRoom(ctx, nil, roomNID)
}

// GetServerInRoom returns true if we think a server is in a given room or false otherwise.
func (d *Database) GetServerInRoom(ctx context.Context, roomNID types.RoomNID, serverName gomatrixserverlib.ServerName) (bool, error) {
	return d.MembershipTable.SelectServerInRoom(ctx, nil, roomNID, serverName)
}

// GetKnownUsers searches all users that userID knows about.
func (d *Database) GetKnownUsers(ctx context.Context, userID, searchString string, limit int) ([]string, error) {
	stateKeyNID, err := d.EventStateKeysTable.SelectEventStateKeyNID(ctx, nil, userID)
	if err != nil {
		return nil, err
	}
	return d.MembershipTable.SelectKnownUsers(ctx, nil, stateKeyNID, searchString, limit)
}

// GetKnownRooms returns a list of all rooms we know about.
func (d *Database) GetKnownRooms(ctx context.Context) ([]string, error) {
	return d.RoomsTable.SelectRoomIDsWithEvents(ctx, nil)
}

// ForgetRoom sets a users room to forgotten
func (d *Database) ForgetRoom(ctx context.Context, userID, roomID string, forget bool) error {
	roomNIDs, err := d.RoomsTable.BulkSelectRoomNIDs(ctx, nil, []string{roomID})
	if err != nil {
		return err
	}
	if len(roomNIDs) > 1 {
		return fmt.Errorf("expected one room, got %d", len(roomNIDs))
	}
	stateKeyNID, err := d.EventStateKeysTable.SelectEventStateKeyNID(ctx, nil, userID)
	if err != nil {
		return err
	}

	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.MembershipTable.UpdateForgetMembership(ctx, nil, roomNIDs[0], stateKeyNID, forget)
	})
}

// PurgeRoom removes all information about a given room from the roomserver.
// For large rooms this operation may take a considerable amount of time.
func (d *Database) PurgeRoom(ctx context.Context, roomID string) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		roomNID, err := d.RoomsTable.SelectRoomNIDForUpdate(ctx, txn, roomID)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("room %s does not exist", roomID)
			}
			return fmt.Errorf("failed to lock the room: %w", err)
		}
		return d.Purge.PurgeRoom(ctx, txn, roomNID, roomID)
	})
}

func (d *Database) UpgradeRoom(ctx context.Context, oldRoomID, newRoomID, eventSender string) error {

	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		published, err := d.PublishedTable.SelectPublishedFromRoomID(ctx, txn, oldRoomID)
		if err != nil {
			return fmt.Errorf("failed to get published room: %w", err)
		}
		if published {
			// un-publish old room
			if err = d.PublishedTable.UpsertRoomPublished(ctx, txn, oldRoomID, "", "", false); err != nil {
				return fmt.Errorf("failed to unpublish room: %w", err)
			}
			// publish new room
			if err = d.PublishedTable.UpsertRoomPublished(ctx, txn, newRoomID, "", "", true); err != nil {
				return fmt.Errorf("failed to publish room: %w", err)
			}
		}

		// Migrate any existing room aliases
		aliases, err := d.RoomAliasesTable.SelectAliasesFromRoomID(ctx, txn, oldRoomID)
		if err != nil {
			return fmt.Errorf("failed to get room aliases: %w", err)
		}

		for _, alias := range aliases {
			if err = d.RoomAliasesTable.DeleteRoomAlias(ctx, txn, alias); err != nil {
				return fmt.Errorf("failed to remove room alias: %w", err)
			}
			if err = d.RoomAliasesTable.InsertRoomAlias(ctx, txn, alias, newRoomID, eventSender); err != nil {
				return fmt.Errorf("failed to set room alias: %w", err)
			}
		}
		return nil
	})
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
			panic(fmt.Errorf("corrupt DB: Missing state block numeric ID %d", stateBlockNID))
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
