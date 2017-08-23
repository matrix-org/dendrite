// Copyright 2017 Vector Creations Ltd
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
	"database/sql"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	statements statements
	db         *sql.DB
}

// Open a postgres database.
func Open(dataSourceName string) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	if err = d.statements.prepare(d.db); err != nil {
		return nil, err
	}
	return &d, nil
}

// StoreEvent implements input.EventDatabase
func (d *Database) StoreEvent(event gomatrixserverlib.Event, authEventNIDs []types.EventNID) (types.RoomNID, types.StateAtEvent, error) {
	var (
		roomNID          types.RoomNID
		eventTypeNID     types.EventTypeNID
		eventStateKeyNID types.EventStateKeyNID
		eventNID         types.EventNID
		stateNID         types.StateSnapshotNID
		err              error
	)

	if roomNID, err = d.assignRoomNID(nil, event.RoomID()); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	if eventTypeNID, err = d.assignEventTypeNID(event.Type()); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	eventStateKey := event.StateKey()
	// Assigned a numeric ID for the state_key if there is one present.
	// Otherwise set the numeric ID for the state_key to 0.
	if eventStateKey != nil {
		if eventStateKeyNID, err = d.assignStateKeyNID(nil, *eventStateKey); err != nil {
			return 0, types.StateAtEvent{}, err
		}
	}

	if eventNID, stateNID, err = d.statements.insertEvent(
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
			eventNID, stateNID, err = d.statements.selectEvent(event.EventID())
		}
		if err != nil {
			return 0, types.StateAtEvent{}, err
		}
	}

	if err = d.statements.insertEventJSON(eventNID, event.JSON()); err != nil {
		return 0, types.StateAtEvent{}, err
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
	}, nil
}

func (d *Database) assignRoomNID(txn *sql.Tx, roomID string) (types.RoomNID, error) {
	// Check if we already have a numeric ID in the database.
	roomNID, err := d.statements.selectRoomNID(txn, roomID)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		roomNID, err = d.statements.insertRoomNID(txn, roomID)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			roomNID, err = d.statements.selectRoomNID(txn, roomID)
		}
	}
	return roomNID, err
}

func (d *Database) assignEventTypeNID(eventType string) (types.EventTypeNID, error) {
	// Check if we already have a numeric ID in the database.
	eventTypeNID, err := d.statements.selectEventTypeNID(eventType)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventTypeNID, err = d.statements.insertEventTypeNID(eventType)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventTypeNID, err = d.statements.selectEventTypeNID(eventType)
		}
	}
	return eventTypeNID, err
}

func (d *Database) assignStateKeyNID(txn *sql.Tx, eventStateKey string) (types.EventStateKeyNID, error) {
	// Check if we already have a numeric ID in the database.
	eventStateKeyNID, err := d.statements.selectEventStateKeyNID(txn, eventStateKey)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventStateKeyNID, err = d.statements.insertEventStateKeyNID(txn, eventStateKey)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventStateKeyNID, err = d.statements.selectEventStateKeyNID(txn, eventStateKey)
		}
	}
	return eventStateKeyNID, err
}

// StateEntriesForEventIDs implements input.EventDatabase
func (d *Database) StateEntriesForEventIDs(eventIDs []string) ([]types.StateEntry, error) {
	return d.statements.bulkSelectStateEventByID(eventIDs)
}

// EventTypeNIDs implements state.RoomStateDatabase
func (d *Database) EventTypeNIDs(eventTypes []string) (map[string]types.EventTypeNID, error) {
	return d.statements.bulkSelectEventTypeNID(eventTypes)
}

// EventStateKeyNIDs implements state.RoomStateDatabase
func (d *Database) EventStateKeyNIDs(eventStateKeys []string) (map[string]types.EventStateKeyNID, error) {
	return d.statements.bulkSelectEventStateKeyNID(eventStateKeys)
}

// EventStateKeys implements query.RoomserverQueryAPIDatabase
func (d *Database) EventStateKeys(eventStateKeyNIDs []types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error) {
	return d.statements.bulkSelectEventStateKey(eventStateKeyNIDs)
}

// EventNIDs implements query.RoomserverQueryAPIDatabase
func (d *Database) EventNIDs(eventIDs []string) (map[string]types.EventNID, error) {
	return d.statements.bulkSelectEventNID(eventIDs)
}

// Events implements input.EventDatabase
func (d *Database) Events(eventNIDs []types.EventNID) ([]types.Event, error) {
	eventJSONs, err := d.statements.bulkSelectEventJSON(eventNIDs)
	if err != nil {
		return nil, err
	}
	results := make([]types.Event, len(eventJSONs))
	for i, eventJSON := range eventJSONs {
		result := &results[i]
		result.EventNID = eventJSON.EventNID
		// TODO: Use NewEventFromTrustedJSON for efficiency
		result.Event, err = gomatrixserverlib.NewEventFromUntrustedJSON(eventJSON.EventJSON)
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

// AddState implements input.EventDatabase
func (d *Database) AddState(roomNID types.RoomNID, stateBlockNIDs []types.StateBlockNID, state []types.StateEntry) (types.StateSnapshotNID, error) {
	if len(state) > 0 {
		stateBlockNID, err := d.statements.selectNextStateBlockNID()
		if err != nil {
			return 0, err
		}
		if err = d.statements.bulkInsertStateData(stateBlockNID, state); err != nil {
			return 0, err
		}
		stateBlockNIDs = append(stateBlockNIDs[:len(stateBlockNIDs):len(stateBlockNIDs)], stateBlockNID)
	}

	return d.statements.insertState(roomNID, stateBlockNIDs)
}

// SetState implements input.EventDatabase
func (d *Database) SetState(eventNID types.EventNID, stateNID types.StateSnapshotNID) error {
	return d.statements.updateEventState(eventNID, stateNID)
}

// StateAtEventIDs implements input.EventDatabase
func (d *Database) StateAtEventIDs(eventIDs []string) ([]types.StateAtEvent, error) {
	return d.statements.bulkSelectStateAtEventByID(eventIDs)
}

// StateBlockNIDs implements state.RoomStateDatabase
func (d *Database) StateBlockNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error) {
	return d.statements.bulkSelectStateBlockNIDs(stateNIDs)
}

// StateEntries implements state.RoomStateDatabase
func (d *Database) StateEntries(stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error) {
	return d.statements.bulkSelectStateBlockEntries(stateBlockNIDs)
}

// EventIDs implements input.RoomEventDatabase
func (d *Database) EventIDs(eventNIDs []types.EventNID) (map[types.EventNID]string, error) {
	return d.statements.bulkSelectEventID(eventNIDs)
}

// GetLatestEventsForUpdate implements input.EventDatabase
func (d *Database) GetLatestEventsForUpdate(roomNID types.RoomNID) (types.RoomRecentEventsUpdater, error) {
	txn, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	eventNIDs, lastEventNIDSent, currentStateSnapshotNID, err := d.statements.selectLatestEventsNIDsForUpdate(txn, roomNID)
	if err != nil {
		txn.Rollback()
		return nil, err
	}
	stateAndRefs, err := d.statements.bulkSelectStateAtEventAndReference(txn, eventNIDs)
	if err != nil {
		txn.Rollback()
		return nil, err
	}
	var lastEventIDSent string
	if lastEventNIDSent != 0 {
		lastEventIDSent, err = d.statements.selectEventID(txn, lastEventNIDSent)
		if err != nil {
			txn.Rollback()
			return nil, err
		}
	}
	return &roomRecentEventsUpdater{
		transaction{txn}, d, roomNID, stateAndRefs, lastEventIDSent, currentStateSnapshotNID,
	}, nil
}

type roomRecentEventsUpdater struct {
	transaction
	d                       *Database
	roomNID                 types.RoomNID
	latestEvents            []types.StateAtEventAndReference
	lastEventIDSent         string
	currentStateSnapshotNID types.StateSnapshotNID
}

// LatestEvents implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) LatestEvents() []types.StateAtEventAndReference {
	return u.latestEvents
}

// LastEventIDSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) LastEventIDSent() string {
	return u.lastEventIDSent
}

// CurrentStateSnapshotNID implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) CurrentStateSnapshotNID() types.StateSnapshotNID {
	return u.currentStateSnapshotNID
}

// StorePreviousEvents implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) StorePreviousEvents(eventNID types.EventNID, previousEventReferences []gomatrixserverlib.EventReference) error {
	for _, ref := range previousEventReferences {
		if err := u.d.statements.insertPreviousEvent(u.txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
			return err
		}
	}
	return nil
}

// IsReferenced implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (bool, error) {
	err := u.d.statements.selectPreviousEventExists(u.txn, eventReference.EventID, eventReference.EventSHA256)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

// SetLatestEvents implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) SetLatestEvents(
	roomNID types.RoomNID, latest []types.StateAtEventAndReference, lastEventNIDSent types.EventNID,
	currentStateSnapshotNID types.StateSnapshotNID,
) error {
	eventNIDs := make([]types.EventNID, len(latest))
	for i := range latest {
		eventNIDs[i] = latest[i].EventNID
	}
	return u.d.statements.updateLatestEventNIDs(u.txn, roomNID, eventNIDs, lastEventNIDSent, currentStateSnapshotNID)
}

// HasEventBeenSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) HasEventBeenSent(eventNID types.EventNID) (bool, error) {
	return u.d.statements.selectEventSentToOutput(u.txn, eventNID)
}

// MarkEventAsSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) MarkEventAsSent(eventNID types.EventNID) error {
	return u.d.statements.updateEventSentToOutput(u.txn, eventNID)
}

func (u *roomRecentEventsUpdater) MembershipUpdater(targetUserNID types.EventStateKeyNID) (types.MembershipUpdater, error) {
	return u.d.membershipUpdaterTxn(u.txn, u.roomNID, targetUserNID)
}

// RoomNID implements query.RoomserverQueryAPIDB
func (d *Database) RoomNID(roomID string) (types.RoomNID, error) {
	roomNID, err := d.statements.selectRoomNID(nil, roomID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return roomNID, err
}

// LatestEventIDs implements query.RoomserverQueryAPIDB
func (d *Database) LatestEventIDs(roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, int64, error) {
	eventNIDs, currentStateSnapshotNID, err := d.statements.selectLatestEventNIDs(roomNID)
	if err != nil {
		return nil, 0, 0, err
	}
	references, err := d.statements.bulkSelectEventReference(eventNIDs)
	if err != nil {
		return nil, 0, 0, err
	}
	depth, err := d.statements.selectMaxEventDepth(eventNIDs)
	if err != nil {
		return nil, 0, 0, err
	}
	return references, currentStateSnapshotNID, depth, nil
}

// GetInvitesForUser implements query.RoomserverQueryAPIDB
func (d *Database) GetInvitesForUser(
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (senderUserIDs []types.EventStateKeyNID, err error) {
	return d.statements.selectInviteActiveForUserInRoom(targetUserNID, roomNID)
}

// SetRoomAlias implements alias.RoomserverAliasAPIDB
func (d *Database) SetRoomAlias(alias string, roomID string) error {
	return d.statements.insertRoomAlias(alias, roomID)
}

// GetRoomIDFromAlias implements alias.RoomserverAliasAPIDB
func (d *Database) GetRoomIDFromAlias(alias string) (string, error) {
	return d.statements.selectRoomIDFromAlias(alias)
}

// GetAliasesFromRoomID implements alias.RoomserverAliasAPIDB
func (d *Database) GetAliasesFromRoomID(roomID string) ([]string, error) {
	return d.statements.selectAliasesFromRoomID(roomID)
}

// RemoveRoomAlias implements alias.RoomserverAliasAPIDB
func (d *Database) RemoveRoomAlias(alias string) error {
	return d.statements.deleteRoomAlias(alias)
}

// StateEntriesForTuples implements state.RoomStateDatabase
func (d *Database) StateEntriesForTuples(
	stateBlockNIDs []types.StateBlockNID, stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	return d.statements.bulkSelectFilteredStateBlockEntries(stateBlockNIDs, stateKeyTuples)
}

// MembershipUpdater implements input.RoomEventDatabase
func (d *Database) MembershipUpdater(roomID, targetUserID string) (types.MembershipUpdater, error) {
	txn, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			txn.Rollback()
		}
	}()

	roomNID, err := d.assignRoomNID(txn, roomID)
	if err != nil {
		return nil, err
	}

	targetUserNID, err := d.assignStateKeyNID(txn, targetUserID)
	if err != nil {
		return nil, err
	}

	updater, err := d.membershipUpdaterTxn(txn, roomNID, targetUserNID)
	if err != nil {
		return nil, err
	}

	succeeded = true
	return updater, nil
}

type membershipUpdater struct {
	transaction
	d             *Database
	roomNID       types.RoomNID
	targetUserNID types.EventStateKeyNID
	membership    membershipState
}

func (d *Database) membershipUpdaterTxn(
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (types.MembershipUpdater, error) {

	if err := d.statements.insertMembership(txn, roomNID, targetUserNID); err != nil {
		return nil, err
	}

	membership, err := d.statements.selectMembershipForUpdate(txn, roomNID, targetUserNID)
	if err != nil {
		return nil, err
	}

	return &membershipUpdater{
		transaction{txn}, d, roomNID, targetUserNID, membership,
	}, nil
}

// IsInvite implements types.MembershipUpdater
func (u *membershipUpdater) IsInvite() bool {
	return u.membership == membershipStateInvite
}

// IsJoin implements types.MembershipUpdater
func (u *membershipUpdater) IsJoin() bool {
	return u.membership == membershipStateJoin
}

// IsLeave implements types.MembershipUpdater
func (u *membershipUpdater) IsLeave() bool {
	return u.membership == membershipStateLeaveOrBan
}

// SetToInvite implements types.MembershipUpdater
func (u *membershipUpdater) SetToInvite(event gomatrixserverlib.Event) (bool, error) {
	senderUserNID, err := u.d.assignStateKeyNID(u.txn, event.Sender())
	if err != nil {
		return false, err
	}
	inserted, err := u.d.statements.insertInviteEvent(
		u.txn, event.EventID(), u.roomNID, u.targetUserNID, senderUserNID, event.JSON(),
	)
	if err != nil {
		return false, err
	}
	if u.membership != membershipStateInvite {
		if err = u.d.statements.updateMembership(
			u.txn, u.roomNID, u.targetUserNID, senderUserNID, membershipStateInvite, 0,
		); err != nil {
			return false, err
		}
	}
	return inserted, nil
}

// SetToJoin implements types.MembershipUpdater
func (u *membershipUpdater) SetToJoin(senderUserID string, eventID string, isUpdate bool) ([]string, error) {
	var inviteEventIDs []string

	senderUserNID, err := u.d.assignStateKeyNID(u.txn, senderUserID)
	if err != nil {
		return nil, err
	}

	// If this is a join event update, there is no invite to update
	if !isUpdate {
		inviteEventIDs, err = u.d.statements.updateInviteRetired(
			u.txn, u.roomNID, u.targetUserNID,
		)
		if err != nil {
			return nil, err
		}
	}

	// Look up the NID of the new join event
	nIDs, err := u.d.EventNIDs([]string{eventID})
	if err != nil {
		return nil, err
	}

	if u.membership != membershipStateJoin || isUpdate {
		if err = u.d.statements.updateMembership(
			u.txn, u.roomNID, u.targetUserNID, senderUserNID, membershipStateJoin, nIDs[eventID],
		); err != nil {
			return nil, err
		}
	}

	return inviteEventIDs, nil
}

// SetToLeave implements types.MembershipUpdater
func (u *membershipUpdater) SetToLeave(senderUserID string, eventID string) ([]string, error) {
	senderUserNID, err := u.d.assignStateKeyNID(u.txn, senderUserID)
	if err != nil {
		return nil, err
	}
	inviteEventIDs, err := u.d.statements.updateInviteRetired(
		u.txn, u.roomNID, u.targetUserNID,
	)
	if err != nil {
		return nil, err
	}

	// Look up the NID of the new leave event
	nIDs, err := u.d.EventNIDs([]string{eventID})
	if err != nil {
		return nil, err
	}

	if u.membership != membershipStateLeaveOrBan {
		if err = u.d.statements.updateMembership(
			u.txn, u.roomNID, u.targetUserNID, senderUserNID, membershipStateLeaveOrBan, nIDs[eventID],
		); err != nil {
			return nil, err
		}
	}
	return inviteEventIDs, nil
}

// GetMembershipEvents implements query.RoomserverQueryAPIDB
func (d *Database) GetMembershipEvents(roomNID types.RoomNID, requestSenderUserID string) (events []types.Event, err error) {
	txn, err := d.db.Begin()
	if err != nil {
		return
	}
	defer txn.Commit()

	requestSenderUserNID, err := d.assignStateKeyNID(txn, requestSenderUserID)
	if err != nil {
		return
	}

	_, senderMembership, err := d.statements.selectMembershipFromRoomAndTarget(roomNID, requestSenderUserNID)
	if err == sql.ErrNoRows {
		// The user has never been a member of that room
		return nil, nil
	} else if err != nil {
		return
	}

	if senderMembership == membershipStateJoin {
		// The user is still in the room: Send the current list of joined members
		var joinEventNIDs []types.EventNID
		joinEventNIDs, err = d.statements.selectMembershipsFromRoomAndMembership(roomNID, membershipStateJoin)
		if err != nil {
			return nil, err
		}

		events, err = d.Events(joinEventNIDs)
	} else {
		// The user isn't in the room anymore
		// TODO: Send the list of joined member as it was when the user left
		//       We cannot do this using only the memberships database, as it
		//       only stores the latest join event NID for a given target user.
		//       The solution would be to build the state of a room after before
		//       the leave event and extract a members list from it.
		//       For now, we return an empty slice so we know the user has been
		//       in the room before.
		events = []types.Event{}
	}

	return
}

type transaction struct {
	txn *sql.Tx
}

// Commit implements types.Transaction
func (t *transaction) Commit() error {
	return t.txn.Commit()
}

// Rollback implements types.Transaction
func (t *transaction) Rollback() error {
	return t.txn.Rollback()
}
