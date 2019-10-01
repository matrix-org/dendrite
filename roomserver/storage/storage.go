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
	"context"
	"database/sql"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
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
func (d *Database) StoreEvent(
	ctx context.Context, event gomatrixserverlib.Event,
	txnAndSessionID *api.TransactionID, authEventNIDs []types.EventNID,
) (types.RoomNID, types.StateAtEvent, error) {
	var (
		roomNID          types.RoomNID
		eventTypeNID     types.EventTypeNID
		eventStateKeyNID types.EventStateKeyNID
		eventNID         types.EventNID
		stateNID         types.StateSnapshotNID
		err              error
	)

	if txnAndSessionID != nil {
		if err = d.statements.insertTransaction(
			ctx, txnAndSessionID.TransactionID,
			txnAndSessionID.SessionID, event.Sender(), event.EventID(),
		); err != nil {
			return 0, types.StateAtEvent{}, err
		}
	}

	if roomNID, err = d.assignRoomNID(ctx, nil, event.RoomID()); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	if eventTypeNID, err = d.assignEventTypeNID(ctx, event.Type()); err != nil {
		return 0, types.StateAtEvent{}, err
	}

	eventStateKey := event.StateKey()
	// Assigned a numeric ID for the state_key if there is one present.
	// Otherwise set the numeric ID for the state_key to 0.
	if eventStateKey != nil {
		if eventStateKeyNID, err = d.assignStateKeyNID(ctx, nil, *eventStateKey); err != nil {
			return 0, types.StateAtEvent{}, err
		}
	}

	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		if eventNID, stateNID, err = d.statements.insertEvent(
			ctx, txn,
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
				eventNID, stateNID, err = d.statements.selectEvent(ctx, txn, event.EventID())
			}
			if err != nil {
				return err
			}
		}

		if err = d.updateSpecialTablesForEvent(
			ctx, txn, &event, eventNID,
		); err != nil {
			return err
		}

		return d.statements.insertEventJSON(ctx, txn, eventNID, event.JSON())
	})
	if err != nil {
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

func (d *Database) assignRoomNID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (types.RoomNID, error) {
	// Check if we already have a numeric ID in the database.
	roomNID, err := d.statements.selectRoomNID(ctx, txn, roomID)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		roomNID, err = d.statements.insertRoomNID(ctx, txn, roomID)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			roomNID, err = d.statements.selectRoomNID(ctx, txn, roomID)
		}
	}
	return roomNID, err
}

func (d *Database) assignEventTypeNID(
	ctx context.Context, eventType string,
) (types.EventTypeNID, error) {
	// Check if we already have a numeric ID in the database.
	eventTypeNID, err := d.statements.selectEventTypeNID(ctx, eventType)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventTypeNID, err = d.statements.insertEventTypeNID(ctx, eventType)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventTypeNID, err = d.statements.selectEventTypeNID(ctx, eventType)
		}
	}
	return eventTypeNID, err
}

func (d *Database) assignStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	// Check if we already have a numeric ID in the database.
	eventStateKeyNID, err := d.statements.selectEventStateKeyNID(ctx, txn, eventStateKey)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventStateKeyNID, err = d.statements.insertEventStateKeyNID(ctx, txn, eventStateKey)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventStateKeyNID, err = d.statements.selectEventStateKeyNID(ctx, txn, eventStateKey)
		}
	}
	return eventStateKeyNID, err
}

func (d *Database) updateSpecialTablesForEvent(
	ctx context.Context,
	txn *sql.Tx,
	event *gomatrixserverlib.Event,
	eventNID types.EventNID,
) (err error) {
	switch event.Type() {
	case gomatrixserverlib.MRoomRedaction:
		// TODO: After we support room versioning, set validated = false only for rooms >= v3.
		if err = d.statements.insertRedaction(
			ctx, txn, eventNID, event.Redacts(), false,
		); err != nil {
			return err
		}
	}
	return nil
}

// StateEntriesForEventIDs implements input.EventDatabase
func (d *Database) StateEntriesForEventIDs(
	ctx context.Context, eventIDs []string,
) ([]types.StateEntry, error) {
	return d.statements.bulkSelectStateEventByID(ctx, eventIDs)
}

// EventTypeNIDs implements state.RoomStateDatabase
func (d *Database) EventTypeNIDs(
	ctx context.Context, eventTypes []string,
) (map[string]types.EventTypeNID, error) {
	return d.statements.bulkSelectEventTypeNID(ctx, eventTypes)
}

// EventStateKeyNIDs implements state.RoomStateDatabase
func (d *Database) EventStateKeyNIDs(
	ctx context.Context, eventStateKeys []string,
) (map[string]types.EventStateKeyNID, error) {
	return d.statements.bulkSelectEventStateKeyNID(ctx, eventStateKeys)
}

// EventStateKeys implements query.RoomserverQueryAPIDatabase
func (d *Database) EventStateKeys(
	ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID,
) (map[types.EventStateKeyNID]string, error) {
	return d.statements.bulkSelectEventStateKey(ctx, eventStateKeyNIDs)
}

// EventNIDs implements query.RoomserverQueryAPIDatabase
func (d *Database) EventNIDs(
	ctx context.Context, eventIDs []string,
) (map[string]types.EventNID, error) {
	return d.statements.bulkSelectEventNID(ctx, eventIDs)
}

// Events implements input.EventDatabase
func (d *Database) Events(
	ctx context.Context, eventNIDs []types.EventNID,
) ([]types.Event, error) {
	eventJSONs, err := d.statements.bulkSelectEventJSON(ctx, eventNIDs)
	if err != nil {
		return nil, err
	}

	results := make([]types.Event, len(eventJSONs))
	eventPointers := make([]*gomatrixserverlib.Event, len(eventJSONs))
	for i, eventJSON := range eventJSONs {
		result := &results[i]
		result.EventNID = eventJSON.EventNID
		// TODO: Use NewEventFromTrustedJSON for efficiency
		result.Event, err = gomatrixserverlib.NewEventFromUntrustedJSON(eventJSON.EventJSON)
		if err != nil {
			return nil, err
		}
		eventPointers[i] = &result.Event
	}

	if err = d.applyRedactions(ctx, eventPointers); err != nil {
		return nil, err
	}

	return results, nil
}

// applyRedactions applies necessary redactions to the given events.
// It will replace events referenced by the pointers with their redacted versions.
// It will update the validation status in the redactions table if there are
// redaction events newly validated.
func (d *Database) applyRedactions(
	ctx context.Context,
	eventPointers []*gomatrixserverlib.Event,
) error {
	eventIDs := make([]string, len(eventPointers))
	for i, e := range eventPointers {
		eventIDs[i] = e.EventID()
	}

	validatedRedactions, unvalidatedRedactions, err := d.statements.bulkSelectRedaction(ctx, nil, eventIDs)
	if err != nil {
		return err
	}

	totalPossibleRedactions := len(validatedRedactions) + len(unvalidatedRedactions)

	// Fast path if nothing to redact
	if totalPossibleRedactions == 0 {
		return nil
	}

	redactionNIDToEvent, err := d.fetchRedactionEvents(ctx, validatedRedactions, unvalidatedRedactions)
	if err != nil {
		return err
	}

	eventIDToEventPointer := make(map[string]*gomatrixserverlib.Event, len(eventPointers))
	for _, p := range eventPointers {
		eventIDToEventPointer[p.EventID()] = p
	}

	if len(unvalidatedRedactions) != 0 {
		var newlyValidated redactedToRedactionMap
		if newlyValidated, err = d.validateRedactions(
			ctx, unvalidatedRedactions, redactionNIDToEvent, eventIDToEventPointer,
		); err != nil {
			return err
		}
		for redactedEventID, redactedByNID := range newlyValidated {
			validatedRedactions[redactedEventID] = redactedByNID
		}
	}

	for redactedEventID, redactedByNID := range validatedRedactions {
		redactedEvent := eventIDToEventPointer[redactedEventID]
		*redactedEvent = redactedEvent.Redact()
		if err = redactedEvent.SetUnsignedField(
			"redacted_because",
			gomatrixserverlib.ToClientEvent(
				*redactionNIDToEvent[redactedByNID],
				gomatrixserverlib.FormatAll,
			),
		); err != nil {
			return err
		}
	}

	return nil
}

func (d *Database) fetchRedactionEvents(
	ctx context.Context,
	validatedRedactions, unvalidatedRedactions redactedToRedactionMap,
) (redactionNIDToEvent map[types.EventNID]*gomatrixserverlib.Event, err error) {
	redactionEventsToFetch := make([]types.EventNID, 0, len(validatedRedactions)+len(unvalidatedRedactions))
	for _, nid := range validatedRedactions {
		redactionEventsToFetch = append(redactionEventsToFetch, nid)
	}
	for _, nid := range unvalidatedRedactions {
		redactionEventsToFetch = append(redactionEventsToFetch, nid)
	}

	redactionJSONs, err := d.statements.bulkSelectEventJSON(ctx, redactionEventsToFetch)
	if err != nil {
		return nil, err
	}

	redactionNIDToEvent = make(map[types.EventNID]*gomatrixserverlib.Event, len(redactionJSONs))
	for _, redactionJSON := range redactionJSONs {
		e, err := gomatrixserverlib.NewEventFromTrustedJSON(redactionJSON.EventJSON, false)
		if err != nil {
			return nil, err
		}
		redactionNIDToEvent[redactionJSON.EventNID] = &e
	}

	return
}

// validateRedactions attempts to validate the given redactions.
// Returns redactions that are successfully validated.
// It will update the validation status in the database for the validated redactions.
func (d *Database) validateRedactions(
	ctx context.Context,
	unvalidatedRedactions redactedToRedactionMap,
	redactionNIDToEvent map[types.EventNID]*gomatrixserverlib.Event,
	redactedIDToEvent map[string]*gomatrixserverlib.Event,
) (validatedRedactions redactedToRedactionMap, err error) {
	validatedRedactions = make(redactedToRedactionMap, len(unvalidatedRedactions))

	for redactedEventID, redactedByNID := range unvalidatedRedactions {
		badEvents, needPowerLevelCheck, validationErr := common.ValidateRedaction(
			redactedIDToEvent[redactedEventID], redactionNIDToEvent[redactedByNID],
		)
		if validationErr != nil {
			return nil, validationErr
		}
		if badEvents {
			continue
		}

		if needPowerLevelCheck {
			// TODO: Still allow power users to redact
			continue
		}

		validatedRedactions[redactedEventID] = redactedByNID
	}

	eventNIDs := make([]types.EventNID, 0, len(validatedRedactions))
	for _, nid := range validatedRedactions {
		eventNIDs = append(eventNIDs, nid)
	}
	if err = d.statements.bulkUpdateValidationStatus(
		ctx, nil, eventNIDs, true,
	); err != nil {
		return nil, err
	}

	// TODO: We might want to clear the unvalidated redactions

	return validatedRedactions, nil
}

// AddState implements input.EventDatabase
func (d *Database) AddState(
	ctx context.Context,
	roomNID types.RoomNID,
	stateBlockNIDs []types.StateBlockNID,
	state []types.StateEntry,
) (types.StateSnapshotNID, error) {
	if len(state) > 0 {
		stateBlockNID, err := d.statements.selectNextStateBlockNID(ctx)
		if err != nil {
			return 0, err
		}
		if err = d.statements.bulkInsertStateData(ctx, stateBlockNID, state); err != nil {
			return 0, err
		}
		stateBlockNIDs = append(stateBlockNIDs[:len(stateBlockNIDs):len(stateBlockNIDs)], stateBlockNID)
	}

	return d.statements.insertState(ctx, roomNID, stateBlockNIDs)
}

// SetState implements input.EventDatabase
func (d *Database) SetState(
	ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {
	return d.statements.updateEventState(ctx, eventNID, stateNID)
}

// StateAtEventIDs implements input.EventDatabase
func (d *Database) StateAtEventIDs(
	ctx context.Context, eventIDs []string,
) ([]types.StateAtEvent, error) {
	return d.statements.bulkSelectStateAtEventByID(ctx, eventIDs)
}

// StateBlockNIDs implements state.RoomStateDatabase
func (d *Database) StateBlockNIDs(
	ctx context.Context, stateNIDs []types.StateSnapshotNID,
) ([]types.StateBlockNIDList, error) {
	return d.statements.bulkSelectStateBlockNIDs(ctx, stateNIDs)
}

// StateEntries implements state.RoomStateDatabase
func (d *Database) StateEntries(
	ctx context.Context, stateBlockNIDs []types.StateBlockNID,
) ([]types.StateEntryList, error) {
	return d.statements.bulkSelectStateBlockEntries(ctx, stateBlockNIDs)
}

// SnapshotNIDFromEventID implements state.RoomStateDatabase
func (d *Database) SnapshotNIDFromEventID(
	ctx context.Context, eventID string,
) (types.StateSnapshotNID, error) {
	_, stateNID, err := d.statements.selectEvent(ctx, nil, eventID)
	return stateNID, err
}

// EventIDs implements input.RoomEventDatabase
func (d *Database) EventIDs(
	ctx context.Context, eventNIDs []types.EventNID,
) (map[types.EventNID]string, error) {
	return d.statements.bulkSelectEventID(ctx, eventNIDs)
}

// GetLatestEventsForUpdate implements input.EventDatabase
func (d *Database) GetLatestEventsForUpdate(
	ctx context.Context, roomNID types.RoomNID,
) (types.RoomRecentEventsUpdater, error) {
	txn, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	eventNIDs, lastEventNIDSent, currentStateSnapshotNID, err :=
		d.statements.selectLatestEventsNIDsForUpdate(ctx, txn, roomNID)
	if err != nil {
		txn.Rollback() // nolint: errcheck
		return nil, err
	}
	stateAndRefs, err := d.statements.bulkSelectStateAtEventAndReference(ctx, txn, eventNIDs)
	if err != nil {
		txn.Rollback() // nolint: errcheck
		return nil, err
	}
	var lastEventIDSent string
	if lastEventNIDSent != 0 {
		lastEventIDSent, err = d.statements.selectEventID(ctx, txn, lastEventNIDSent)
		if err != nil {
			txn.Rollback() // nolint: errcheck
			return nil, err
		}
	}
	return &roomRecentEventsUpdater{
		transaction{ctx, txn}, d, roomNID, stateAndRefs, lastEventIDSent, currentStateSnapshotNID,
	}, nil
}

// GetTransactionEventID implements input.EventDatabase
func (d *Database) GetTransactionEventID(
	ctx context.Context, transactionID string,
	sessionID int64, userID string,
) (string, error) {
	eventID, err := d.statements.selectTransactionEventID(ctx, transactionID, sessionID, userID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return eventID, err
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
		if err := u.d.statements.insertPreviousEvent(u.ctx, u.txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
			return err
		}
	}
	return nil
}

// IsReferenced implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (bool, error) {
	err := u.d.statements.selectPreviousEventExists(u.ctx, u.txn, eventReference.EventID, eventReference.EventSHA256)
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
	return u.d.statements.updateLatestEventNIDs(u.ctx, u.txn, roomNID, eventNIDs, lastEventNIDSent, currentStateSnapshotNID)
}

// HasEventBeenSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) HasEventBeenSent(eventNID types.EventNID) (bool, error) {
	return u.d.statements.selectEventSentToOutput(u.ctx, u.txn, eventNID)
}

// MarkEventAsSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) MarkEventAsSent(eventNID types.EventNID) error {
	return u.d.statements.updateEventSentToOutput(u.ctx, u.txn, eventNID)
}

func (u *roomRecentEventsUpdater) MembershipUpdater(targetUserNID types.EventStateKeyNID) (types.MembershipUpdater, error) {
	return u.d.membershipUpdaterTxn(u.ctx, u.txn, u.roomNID, targetUserNID)
}

// RoomNID implements query.RoomserverQueryAPIDB
func (d *Database) RoomNID(ctx context.Context, roomID string) (types.RoomNID, error) {
	roomNID, err := d.statements.selectRoomNID(ctx, nil, roomID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return roomNID, err
}

// LatestEventIDs implements query.RoomserverQueryAPIDatabase
func (d *Database) LatestEventIDs(
	ctx context.Context, roomNID types.RoomNID,
) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, int64, error) {
	eventNIDs, currentStateSnapshotNID, err := d.statements.selectLatestEventNIDs(ctx, roomNID)
	if err != nil {
		return nil, 0, 0, err
	}
	references, err := d.statements.bulkSelectEventReference(ctx, eventNIDs)
	if err != nil {
		return nil, 0, 0, err
	}
	depth, err := d.statements.selectMaxEventDepth(ctx, eventNIDs)
	if err != nil {
		return nil, 0, 0, err
	}
	return references, currentStateSnapshotNID, depth, nil
}

// GetInvitesForUser implements query.RoomserverQueryAPIDatabase
func (d *Database) GetInvitesForUser(
	ctx context.Context,
	roomNID types.RoomNID,
	targetUserNID types.EventStateKeyNID,
) (senderUserIDs []types.EventStateKeyNID, err error) {
	return d.statements.selectInviteActiveForUserInRoom(ctx, targetUserNID, roomNID)
}

// SetRoomAlias implements alias.RoomserverAliasAPIDB
func (d *Database) SetRoomAlias(ctx context.Context, alias string, roomID string, creatorUserID string) error {
	return d.statements.insertRoomAlias(ctx, alias, roomID, creatorUserID)
}

// GetRoomIDForAlias implements alias.RoomserverAliasAPIDB
func (d *Database) GetRoomIDForAlias(ctx context.Context, alias string) (string, error) {
	return d.statements.selectRoomIDFromAlias(ctx, alias)
}

// GetAliasesForRoomID implements alias.RoomserverAliasAPIDB
func (d *Database) GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error) {
	return d.statements.selectAliasesFromRoomID(ctx, roomID)
}

// GetCreatorIDForAlias implements alias.RoomserverAliasAPIDB
func (d *Database) GetCreatorIDForAlias(
	ctx context.Context, alias string,
) (string, error) {
	return d.statements.selectCreatorIDFromAlias(ctx, alias)
}

// RemoveRoomAlias implements alias.RoomserverAliasAPIDB
func (d *Database) RemoveRoomAlias(ctx context.Context, alias string) error {
	return d.statements.deleteRoomAlias(ctx, alias)
}

// StateEntriesForTuples implements state.RoomStateDatabase
func (d *Database) StateEntriesForTuples(
	ctx context.Context,
	stateBlockNIDs []types.StateBlockNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	return d.statements.bulkSelectFilteredStateBlockEntries(
		ctx, stateBlockNIDs, stateKeyTuples,
	)
}

// MembershipUpdater implements input.RoomEventDatabase
func (d *Database) MembershipUpdater(
	ctx context.Context, roomID, targetUserID string,
) (types.MembershipUpdater, error) {
	txn, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			txn.Rollback() // nolint: errcheck
		}
	}()

	roomNID, err := d.assignRoomNID(ctx, txn, roomID)
	if err != nil {
		return nil, err
	}

	targetUserNID, err := d.assignStateKeyNID(ctx, txn, targetUserID)
	if err != nil {
		return nil, err
	}

	updater, err := d.membershipUpdaterTxn(ctx, txn, roomNID, targetUserNID)
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
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	targetUserNID types.EventStateKeyNID,
) (types.MembershipUpdater, error) {

	if err := d.statements.insertMembership(ctx, txn, roomNID, targetUserNID); err != nil {
		return nil, err
	}

	membership, err := d.statements.selectMembershipForUpdate(ctx, txn, roomNID, targetUserNID)
	if err != nil {
		return nil, err
	}

	return &membershipUpdater{
		transaction{ctx, txn}, d, roomNID, targetUserNID, membership,
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
	senderUserNID, err := u.d.assignStateKeyNID(u.ctx, u.txn, event.Sender())
	if err != nil {
		return false, err
	}
	inserted, err := u.d.statements.insertInviteEvent(
		u.ctx, u.txn, event.EventID(), u.roomNID, u.targetUserNID, senderUserNID, event.JSON(),
	)
	if err != nil {
		return false, err
	}
	if u.membership != membershipStateInvite {
		if err = u.d.statements.updateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID, membershipStateInvite, 0,
		); err != nil {
			return false, err
		}
	}
	return inserted, nil
}

// SetToJoin implements types.MembershipUpdater
func (u *membershipUpdater) SetToJoin(senderUserID string, eventID string, isUpdate bool) ([]string, error) {
	var inviteEventIDs []string

	senderUserNID, err := u.d.assignStateKeyNID(u.ctx, u.txn, senderUserID)
	if err != nil {
		return nil, err
	}

	// If this is a join event update, there is no invite to update
	if !isUpdate {
		inviteEventIDs, err = u.d.statements.updateInviteRetired(
			u.ctx, u.txn, u.roomNID, u.targetUserNID,
		)
		if err != nil {
			return nil, err
		}
	}

	// Look up the NID of the new join event
	nIDs, err := u.d.EventNIDs(u.ctx, []string{eventID})
	if err != nil {
		return nil, err
	}

	if u.membership != membershipStateJoin || isUpdate {
		if err = u.d.statements.updateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID,
			membershipStateJoin, nIDs[eventID],
		); err != nil {
			return nil, err
		}
	}

	return inviteEventIDs, nil
}

// SetToLeave implements types.MembershipUpdater
func (u *membershipUpdater) SetToLeave(senderUserID string, eventID string) ([]string, error) {
	senderUserNID, err := u.d.assignStateKeyNID(u.ctx, u.txn, senderUserID)
	if err != nil {
		return nil, err
	}
	inviteEventIDs, err := u.d.statements.updateInviteRetired(
		u.ctx, u.txn, u.roomNID, u.targetUserNID,
	)
	if err != nil {
		return nil, err
	}

	// Look up the NID of the new leave event
	nIDs, err := u.d.EventNIDs(u.ctx, []string{eventID})
	if err != nil {
		return nil, err
	}

	if u.membership != membershipStateLeaveOrBan {
		if err = u.d.statements.updateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID,
			membershipStateLeaveOrBan, nIDs[eventID],
		); err != nil {
			return nil, err
		}
	}
	return inviteEventIDs, nil
}

// GetMembership implements query.RoomserverQueryAPIDB
func (d *Database) GetMembership(
	ctx context.Context, roomNID types.RoomNID, requestSenderUserID string,
) (membershipEventNID types.EventNID, stillInRoom bool, err error) {
	requestSenderUserNID, err := d.assignStateKeyNID(ctx, nil, requestSenderUserID)
	if err != nil {
		return
	}

	senderMembershipEventNID, senderMembership, err :=
		d.statements.selectMembershipFromRoomAndTarget(
			ctx, roomNID, requestSenderUserNID,
		)
	if err == sql.ErrNoRows {
		// The user has never been a member of that room
		return 0, false, nil
	} else if err != nil {
		return
	}

	return senderMembershipEventNID, senderMembership == membershipStateJoin, nil
}

// GetMembershipEventNIDsForRoom implements query.RoomserverQueryAPIDB
func (d *Database) GetMembershipEventNIDsForRoom(
	ctx context.Context, roomNID types.RoomNID, joinOnly bool,
) ([]types.EventNID, error) {
	if joinOnly {
		return d.statements.selectMembershipsFromRoomAndMembership(
			ctx, roomNID, membershipStateJoin,
		)
	}

	return d.statements.selectMembershipsFromRoom(ctx, roomNID)
}

// EventsFromIDs implements query.RoomserverQueryAPIEventDB
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

type transaction struct {
	ctx context.Context
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
