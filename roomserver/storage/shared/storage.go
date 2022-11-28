package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

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
	DB                  *sql.DB
	Cache               caching.RoomServerCaches
	Writer              sqlutil.Writer
	EventsTable         tables.Events
	EventJSONTable      tables.EventJSON
	EventTypesTable     tables.EventTypes
	EventStateKeysTable tables.EventStateKeys
	RoomsTable          tables.Rooms
	StateSnapshotTable  tables.StateSnapshot
	StateBlockTable     tables.StateBlock
	RoomAliasesTable    tables.RoomAliases
	PrevEventsTable     tables.PreviousEvents
	InvitesTable        tables.Invites
	MembershipTable     tables.Membership
	PublishedTable      tables.Published
	RedactionsTable     tables.Redactions
	GetRoomUpdaterFn    func(ctx context.Context, roomInfo *types.RoomInfo) (*RoomUpdater, error)
}

func (d *Database) SupportsConcurrentRoomInputs() bool {
	return true
}

func (d *Database) EventTypeNIDs(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	return d.eventTypeNIDs(ctx, nil, eventTypes)
}

func (d *Database) eventTypeNIDs(
	ctx context.Context, txn *sql.Tx, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	result := make(map[string]types.EventTypeNID)
	nids, err := d.EventTypesTable.BulkSelectEventTypeNID(ctx, txn, eventTypes)
	if err != nil {
		return nil, err
	}
	for eventType, nid := range nids {
		result[eventType] = nid
	}
	return result, nil
}

func (d *Database) EventStateKeys(
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
	fromDB, err := d.EventStateKeysTable.BulkSelectEventStateKey(ctx, nil, fetch)
	if err != nil {
		return nil, err
	}
	for nid, key := range fromDB {
		result[nid] = key
		d.Cache.StoreEventStateKey(nid, key)
	}
	return result, nil
}

func (d *Database) EventStateKeyNIDs(
	ctx context.Context, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	return d.eventStateKeyNIDs(ctx, nil, eventStateKeys)
}

func (d *Database) eventStateKeyNIDs(
	ctx context.Context, txn *sql.Tx, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	result := make(map[string]types.EventStateKeyNID)
	eventStateKeys = util.UniqueStrings(eventStateKeys)
	nids, err := d.EventStateKeysTable.BulkSelectEventStateKeyNID(ctx, txn, eventStateKeys)
	if err != nil {
		return nil, err
	}
	for eventStateKey, nid := range nids {
		result[eventStateKey] = nid
	}
	// We received some nids, but are still missing some, work out which and create them
	if len(eventStateKeys) > len(result) {
		var nid types.EventStateKeyNID
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

func (d *Database) StateEntriesForEventIDs(
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

func (d *Database) EventNIDs(
	ctx context.Context, eventIDs []string,
) (map[string]types.EventNID, error) {
	return d.eventNIDs(ctx, nil, eventIDs, NoFilter)
}

type UnsentFilter bool

const (
	NoFilter         UnsentFilter = false
	FilterUnsentOnly UnsentFilter = true
)

func (d *Database) eventNIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string, filter UnsentFilter,
) (map[string]types.EventNID, error) {
	switch filter {
	case FilterUnsentOnly:
		return d.EventsTable.BulkSelectUnsentEventNID(ctx, txn, eventIDs)
	case NoFilter:
		return d.EventsTable.BulkSelectEventNID(ctx, txn, eventIDs)
	default:
		panic("impossible case")
	}
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
	return d.EventsTable.BulkSelectStateAtEventByID(ctx, nil, eventIDs)
}

func (d *Database) SnapshotNIDFromEventID(
	ctx context.Context, eventID string,
) (types.StateSnapshotNID, error) {
	return d.snapshotNIDFromEventID(ctx, nil, eventID)
}

func (d *Database) snapshotNIDFromEventID(
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

func (d *Database) EventIDs(
	ctx context.Context, eventNIDs []types.EventNID,
) (map[types.EventNID]string, error) {
	return d.EventsTable.BulkSelectEventID(ctx, nil, eventNIDs)
}

func (d *Database) EventsFromIDs(ctx context.Context, eventIDs []string) ([]types.Event, error) {
	return d.eventsFromIDs(ctx, nil, eventIDs, NoFilter)
}

func (d *Database) eventsFromIDs(ctx context.Context, txn *sql.Tx, eventIDs []string, filter UnsentFilter) ([]types.Event, error) {
	nidMap, err := d.eventNIDs(ctx, txn, eventIDs, filter)
	if err != nil {
		return nil, err
	}

	var nids []types.EventNID
	for _, nid := range nidMap {
		nids = append(nids, nid)
	}

	return d.events(ctx, txn, nids)
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

func (d *Database) Events(
	ctx context.Context, eventNIDs []types.EventNID,
) ([]types.Event, error) {
	return d.events(ctx, nil, eventNIDs)
}

func (d *Database) events(
	ctx context.Context, txn *sql.Tx, inputEventNIDs types.EventNIDs,
) ([]types.Event, error) {
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
	eventIDs, _ := d.EventsTable.BulkSelectEventID(ctx, txn, eventNIDs)
	if err != nil {
		eventIDs = map[types.EventNID]string{}
	}
	var roomNIDs map[types.EventNID]types.RoomNID
	roomNIDs, err = d.EventsTable.SelectRoomNIDsForEventNIDs(ctx, txn, eventNIDs)
	if err != nil {
		return nil, err
	}
	uniqueRoomNIDs := make(map[types.RoomNID]struct{})
	for _, n := range roomNIDs {
		uniqueRoomNIDs[n] = struct{}{}
	}
	roomVersions := make(map[types.RoomNID]gomatrixserverlib.RoomVersion)
	fetchNIDList := make([]types.RoomNID, 0, len(uniqueRoomNIDs))
	for n := range uniqueRoomNIDs {
		if roomID, ok := d.Cache.GetRoomServerRoomID(n); ok {
			if roomVersion, ok := d.Cache.GetRoomVersion(roomID); ok {
				roomVersions[n] = roomVersion
				continue
			}
		}
		fetchNIDList = append(fetchNIDList, n)
	}
	dbRoomVersions, err := d.RoomsTable.SelectRoomVersionsForRoomNIDs(ctx, txn, fetchNIDList)
	if err != nil {
		return nil, err
	}
	for n, v := range dbRoomVersions {
		roomVersions[n] = v
	}
	for _, eventJSON := range eventJSONs {
		roomNID := roomNIDs[eventJSON.EventNID]
		roomVersion := roomVersions[roomNID]
		events[eventJSON.EventNID], err = gomatrixserverlib.NewEventFromTrustedJSONWithEventID(
			eventIDs[eventJSON.EventNID], eventJSON.EventJSON, false, roomVersion,
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

func (d *Database) StoreEvent(
	ctx context.Context, event *gomatrixserverlib.Event,
	authEventNIDs []types.EventNID, isRejected bool,
) (types.EventNID, types.RoomNID, types.StateAtEvent, *gomatrixserverlib.Event, string, error) {
	return d.storeEvent(ctx, nil, event, authEventNIDs, isRejected)
}

func (d *Database) storeEvent(
	ctx context.Context, updater *RoomUpdater, event *gomatrixserverlib.Event,
	authEventNIDs []types.EventNID, isRejected bool,
) (types.EventNID, types.RoomNID, types.StateAtEvent, *gomatrixserverlib.Event, string, error) {
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
	var txn *sql.Tx
	if updater != nil && updater.txn != nil {
		txn = updater.txn
	}
	// First writer is with a database-provided transaction, so that NIDs are assigned
	// globally outside of the updater context, to help avoid races.
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
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

		return nil
	})
	if err != nil {
		return 0, 0, types.StateAtEvent{}, nil, "", fmt.Errorf("d.Writer.Do: %w", err)
	}
	// Second writer is using the database-provided transaction, probably from the
	// room updater, for easy roll-back if required.
	err = d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
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
		if !isRejected { // ignore rejected redaction events
			redactionEvent, redactedEventID, err = d.handleRedactions(ctx, txn, eventNID, event)
			if err != nil {
				return fmt.Errorf("d.handleRedactions: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, types.StateAtEvent{}, nil, "", fmt.Errorf("d.Writer.Do: %w", err)
	}

	// We should attempt to update the previous events table with any
	// references that this new event makes. We do this using a latest
	// events updater because it somewhat works as a mutex, ensuring
	// that there's a row-level lock on the latest room events (well,
	// on Postgres at least).
	if prevEvents := event.PrevEvents(); len(prevEvents) > 0 {
		// Create an updater - NB: on sqlite this WILL create a txn as we are directly calling the shared DB form of
		// GetLatestEventsForUpdate - not via the SQLiteDatabase form which has `nil` txns. This
		// function only does SELECTs though so the created txn (at this point) is just a read txn like
		// any other so this is fine. If we ever update GetLatestEventsForUpdate or NewLatestEventsUpdater
		// to do writes however then this will need to go inside `Writer.Do`.
		succeeded := false
		if updater == nil {
			var roomInfo *types.RoomInfo
			roomInfo, err = d.roomInfo(ctx, txn, event.RoomID())
			if err != nil {
				return 0, 0, types.StateAtEvent{}, nil, "", fmt.Errorf("d.RoomInfo: %w", err)
			}
			if roomInfo == nil && len(prevEvents) > 0 {
				return 0, 0, types.StateAtEvent{}, nil, "", fmt.Errorf("expected room %q to exist", event.RoomID())
			}
			updater, err = d.GetRoomUpdater(ctx, roomInfo)
			if err != nil {
				return 0, 0, types.StateAtEvent{}, nil, "", fmt.Errorf("GetRoomUpdater: %w", err)
			}
			defer sqlutil.EndTransactionWithCheck(updater, &succeeded, &err)
		}
		if err = updater.StorePreviousEvents(eventNID, prevEvents); err != nil {
			return 0, 0, types.StateAtEvent{}, nil, "", fmt.Errorf("updater.StorePreviousEvents: %w", err)
		}
		succeeded = true
	}

	return eventNID, roomNID, types.StateAtEvent{
		BeforeStateSnapshotNID: stateNID,
		StateEntry: types.StateEntry{
			StateKeyTuple: types.StateKeyTuple{
				EventTypeNID:     eventTypeNID,
				EventStateKeyNID: eventStateKeyNID,
			},
			EventNID: eventNID,
		},
	}, redactionEvent, redactedEventID, err
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
	return roomNID, err
}

func (d *Database) assignEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventType string,
) (types.EventTypeNID, error) {
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
	return eventTypeNID, err
}

func (d *Database) assignStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
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

// handleRedactions manages the redacted status of events. There's two cases to consider in order to comply with the spec:
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
// Returns the redaction event and the event ID of the redacted event if this call resulted in a redaction.
func (d *Database) handleRedactions(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, event *gomatrixserverlib.Event,
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
			return nil, "", fmt.Errorf("d.RedactionsTable.InsertRedaction: %w", err)
		}
	}

	redactionEvent, redactedEvent, validated, err := d.loadRedactionPair(ctx, txn, eventNID, event)
	if err != nil {
		return nil, "", fmt.Errorf("d.loadRedactionPair: %w", err)
	}
	if validated || redactedEvent == nil || redactionEvent == nil {
		// we've seen this redaction before or there is nothing to redact
		return nil, "", nil
	}
	if redactedEvent.RoomID() != redactionEvent.RoomID() {
		// redactions across rooms aren't allowed
		return nil, "", nil
	}

	// Get the power level from the database, so we can verify the user is allowed to redact the event
	powerLevels, err := d.GetStateEvent(ctx, event.RoomID(), gomatrixserverlib.MRoomPowerLevels, "")
	if err != nil {
		return nil, "", fmt.Errorf("d.GetStateEvent: %w", err)
	}
	if powerLevels == nil {
		return nil, "", fmt.Errorf("unable to fetch m.room.power_levels event from database for room %s", event.RoomID())
	}
	pl, err := powerLevels.PowerLevels()
	if err != nil {
		return nil, "", fmt.Errorf("unable to get powerlevels for room: %w", err)
	}

	redactUser := pl.UserLevel(redactionEvent.Sender())
	switch {
	case redactUser >= pl.Redact:
		// The power level of the redaction event’s sender is greater than or equal to the redact level.
	case redactedEvent.Sender() == redactionEvent.Sender():
		// The domain of the redaction event’s sender matches that of the original event’s sender.
	default:
		return nil, "", nil
	}

	// mark the event as redacted
	if redactionsArePermanent {
		redactedEvent.Redact()
	}

	err = redactedEvent.SetUnsignedField("redacted_because", redactionEvent)
	if err != nil {
		return nil, "", fmt.Errorf("redactedEvent.SetUnsignedField: %w", err)
	}
	// NOTSPEC: sytest relies on this unspecced field existing :(
	err = redactedEvent.SetUnsignedField("redacted_by", redactionEvent.EventID())
	if err != nil {
		return nil, "", fmt.Errorf("redactedEvent.SetUnsignedField: %w", err)
	}
	// overwrite the eventJSON table
	err = d.EventJSONTable.InsertEventJSON(ctx, txn, redactedEvent.EventNID, redactedEvent.JSON())
	if err != nil {
		return nil, "", fmt.Errorf("d.EventJSONTable.InsertEventJSON: %w", err)
	}

	err = d.RedactionsTable.MarkRedactionValidated(ctx, txn, redactionEvent.EventID(), true)
	if err != nil {
		err = fmt.Errorf("d.RedactionsTable.MarkRedactionValidated: %w", err)
	}

	return redactionEvent.Event, redactedEvent.EventID(), err
}

// loadRedactionPair returns both the redaction event and the redacted event, else nil.
func (d *Database) loadRedactionPair(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, event *gomatrixserverlib.Event,
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
			events[i].Redact()
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
	roomInfo, err := d.RoomInfo(ctx, roomID)
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
	roomInfo, err := d.RoomInfo(ctx, roomID)
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
		roomInfo, err2 := d.RoomInfo(ctx, roomID)
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

func (d *Database) UpgradeRoom(ctx context.Context, oldRoomID, newRoomID, eventSender string) error {

	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		// un-publish old room
		if err := d.PublishedTable.UpsertRoomPublished(ctx, txn, oldRoomID, "", "", false); err != nil {
			return fmt.Errorf("failed to unpublish room: %w", err)
		}
		// publish new room
		if err := d.PublishedTable.UpsertRoomPublished(ctx, txn, newRoomID, "", "", true); err != nil {
			return fmt.Errorf("failed to publish room: %w", err)
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
