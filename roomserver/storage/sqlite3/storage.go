// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/matrix-org/dendrite/internal/sqlutil"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	_ "github.com/mattn/go-sqlite3"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	statements statements
	db         *sql.DB
}

// Open a postgres database.
func Open(dataSourceName string) (*Database, error) {
	var d Database
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return nil, err
	}
	var cs string
	if uri.Opaque != "" { // file:filename.db
		cs = uri.Opaque
	} else if uri.Path != "" { // file:///path/to/filename.db
		cs = uri.Path
	} else {
		return nil, errors.New("no filename or path in connect string")
	}
	if d.db, err = sqlutil.Open(common.SQLiteDriverName(), cs); err != nil {
		return nil, err
	}
	//d.db.Exec("PRAGMA journal_mode=WAL;")
	//d.db.Exec("PRAGMA read_uncommitted = true;")

	// FIXME: We are leaking connections somewhere. Setting this to 2 will eventually
	// cause the roomserver to be unresponsive to new events because something will
	// acquire the global mutex and never unlock it because it is waiting for a connection
	// which it will never obtain.
	d.db.SetMaxOpenConns(20)
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

	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		if txnAndSessionID != nil {
			if err = d.statements.insertTransaction(
				ctx, txn, txnAndSessionID.TransactionID,
				txnAndSessionID.SessionID, event.Sender(), event.EventID(),
			); err != nil {
				return err
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
			return err
		}

		if roomNID, err = d.assignRoomNID(ctx, txn, event.RoomID(), roomVersion); err != nil {
			return err
		}

		if eventTypeNID, err = d.assignEventTypeNID(ctx, txn, event.Type()); err != nil {
			return err
		}

		eventStateKey := event.StateKey()
		// Assigned a numeric ID for the state_key if there is one present.
		// Otherwise set the numeric ID for the state_key to 0.
		if eventStateKey != nil {
			if eventStateKeyNID, err = d.assignStateKeyNID(ctx, txn, *eventStateKey); err != nil {
				return err
			}
		}

		if eventNID, err = d.statements.insertEvent(
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
				eventNID, stateNID, err = d.statements.selectEvent(ctx, txn, event.EventID())
			}
			if err != nil {
				return err
			}
		}

		if err = d.statements.insertEventJSON(ctx, txn, eventNID, event.JSON()); err != nil {
			return err
		}

		return nil
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

func (d *Database) assignRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (roomNID types.RoomNID, err error) {
	// Check if we already have a numeric ID in the database.
	roomNID, err = d.statements.selectRoomNID(ctx, txn, roomID)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		roomNID, err = d.statements.insertRoomNID(ctx, txn, roomID, roomVersion)
		if err == nil {
			// Now get the numeric ID back out of the database
			roomNID, err = d.statements.selectRoomNID(ctx, txn, roomID)
		}
	}
	return
}

func (d *Database) assignEventTypeNID(
	ctx context.Context, txn *sql.Tx, eventType string,
) (eventTypeNID types.EventTypeNID, err error) {
	// Check if we already have a numeric ID in the database.
	eventTypeNID, err = d.statements.selectEventTypeNID(ctx, txn, eventType)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventTypeNID, err = d.statements.insertEventTypeNID(ctx, txn, eventType)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventTypeNID, err = d.statements.selectEventTypeNID(ctx, txn, eventType)
		}
	}
	return
}

func (d *Database) assignStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (eventStateKeyNID types.EventStateKeyNID, err error) {
	// Check if we already have a numeric ID in the database.
	eventStateKeyNID, err = d.statements.selectEventStateKeyNID(ctx, txn, eventStateKey)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventStateKeyNID, err = d.statements.insertEventStateKeyNID(ctx, txn, eventStateKey)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventStateKeyNID, err = d.statements.selectEventStateKeyNID(ctx, txn, eventStateKey)
		}
	}
	return
}

// StateEntriesForEventIDs implements input.EventDatabase
func (d *Database) StateEntriesForEventIDs(
	ctx context.Context, eventIDs []string,
) (se []types.StateEntry, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		se, err = d.statements.bulkSelectStateEventByID(ctx, txn, eventIDs)
		return err
	})
	return
}

// EventTypeNIDs implements state.RoomStateDatabase
func (d *Database) EventTypeNIDs(
	ctx context.Context, eventTypes []string,
) (etnids map[string]types.EventTypeNID, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		etnids, err = d.statements.bulkSelectEventTypeNID(ctx, txn, eventTypes)
		return err
	})
	return
}

// EventStateKeyNIDs implements state.RoomStateDatabase
func (d *Database) EventStateKeyNIDs(
	ctx context.Context, eventStateKeys []string,
) (esknids map[string]types.EventStateKeyNID, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		esknids, err = d.statements.bulkSelectEventStateKeyNID(ctx, txn, eventStateKeys)
		return err
	})
	return
}

// EventStateKeys implements query.RoomserverQueryAPIDatabase
func (d *Database) EventStateKeys(
	ctx context.Context, eventStateKeyNIDs []types.EventStateKeyNID,
) (out map[types.EventStateKeyNID]string, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		out, err = d.statements.bulkSelectEventStateKey(ctx, txn, eventStateKeyNIDs)
		return err
	})
	return
}

// EventNIDs implements query.RoomserverQueryAPIDatabase
func (d *Database) EventNIDs(
	ctx context.Context, eventIDs []string,
) (out map[string]types.EventNID, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		out, err = d.statements.bulkSelectEventNID(ctx, txn, eventIDs)
		return err
	})
	return
}

// Events implements input.EventDatabase
func (d *Database) Events(
	ctx context.Context, eventNIDs []types.EventNID,
) ([]types.Event, error) {
	var eventJSONs []eventJSONPair
	var err error
	var results []types.Event
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		eventJSONs, err = d.statements.bulkSelectEventJSON(ctx, txn, eventNIDs)
		if err != nil || len(eventJSONs) == 0 {
			return nil
		}
		results = make([]types.Event, len(eventJSONs))
		for i, eventJSON := range eventJSONs {
			var roomNID types.RoomNID
			var roomVersion gomatrixserverlib.RoomVersion
			result := &results[i]
			result.EventNID = eventJSON.EventNID
			roomNID, err = d.statements.selectRoomNIDForEventNID(ctx, txn, eventJSON.EventNID)
			if err != nil {
				return err
			}
			roomVersion, err = d.statements.selectRoomVersionForRoomNID(ctx, txn, roomNID)
			if err != nil {
				return err
			}
			result.Event, err = gomatrixserverlib.NewEventFromTrustedJSON(
				eventJSON.EventJSON, false, roomVersion,
			)
			if err != nil {
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return []types.Event{}, err
	}
	return results, nil
}

// AddState implements input.EventDatabase
func (d *Database) AddState(
	ctx context.Context,
	roomNID types.RoomNID,
	stateBlockNIDs []types.StateBlockNID,
	state []types.StateEntry,
) (stateNID types.StateSnapshotNID, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		if len(state) > 0 {
			var stateBlockNID types.StateBlockNID
			stateBlockNID, err = d.statements.bulkInsertStateData(ctx, txn, state)
			if err != nil {
				return err
			}
			stateBlockNIDs = append(stateBlockNIDs[:len(stateBlockNIDs):len(stateBlockNIDs)], stateBlockNID)
		}
		stateNID, err = d.statements.insertState(ctx, txn, roomNID, stateBlockNIDs)
		return err
	})
	if err != nil {
		return 0, err
	}
	return
}

// SetState implements input.EventDatabase
func (d *Database) SetState(
	ctx context.Context, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {
	e := common.WithTransaction(d.db, func(txn *sql.Tx) error {
		return d.statements.updateEventState(ctx, txn, eventNID, stateNID)
	})
	return e
}

// StateAtEventIDs implements input.EventDatabase
func (d *Database) StateAtEventIDs(
	ctx context.Context, eventIDs []string,
) (se []types.StateAtEvent, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		se, err = d.statements.bulkSelectStateAtEventByID(ctx, txn, eventIDs)
		return err
	})
	return
}

// StateBlockNIDs implements state.RoomStateDatabase
func (d *Database) StateBlockNIDs(
	ctx context.Context, stateNIDs []types.StateSnapshotNID,
) (sl []types.StateBlockNIDList, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		sl, err = d.statements.bulkSelectStateBlockNIDs(ctx, txn, stateNIDs)
		return err
	})
	return
}

// StateEntries implements state.RoomStateDatabase
func (d *Database) StateEntries(
	ctx context.Context, stateBlockNIDs []types.StateBlockNID,
) (sel []types.StateEntryList, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		sel, err = d.statements.bulkSelectStateBlockEntries(ctx, txn, stateBlockNIDs)
		return err
	})
	return
}

// SnapshotNIDFromEventID implements state.RoomStateDatabase
func (d *Database) SnapshotNIDFromEventID(
	ctx context.Context, eventID string,
) (stateNID types.StateSnapshotNID, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		_, stateNID, err = d.statements.selectEvent(ctx, txn, eventID)
		return err
	})
	return
}

// EventIDs implements input.RoomEventDatabase
func (d *Database) EventIDs(
	ctx context.Context, eventNIDs []types.EventNID,
) (out map[types.EventNID]string, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		out, err = d.statements.bulkSelectEventID(ctx, txn, eventNIDs)
		return err
	})
	return
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

	// FIXME: we probably want to support long-lived txns in sqlite somehow, but we don't because we get
	// 'database is locked' errors caused by multiple write txns (one being the long-lived txn created here)
	// so for now let's not use a long-lived txn at all, and just commit it here and set the txn to nil so
	// we fail fast if someone tries to use the underlying txn object.
	err = txn.Commit()
	if err != nil {
		return nil, err
	}
	return &roomRecentEventsUpdater{
		transaction{ctx, nil}, d, roomNID, stateAndRefs, lastEventIDSent, currentStateSnapshotNID,
	}, nil
}

// GetTransactionEventID implements input.EventDatabase
func (d *Database) GetTransactionEventID(
	ctx context.Context, transactionID string,
	sessionID int64, userID string,
) (string, error) {
	eventID, err := d.statements.selectTransactionEventID(ctx, nil, transactionID, sessionID, userID)
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

// RoomVersion implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) RoomVersion() (version gomatrixserverlib.RoomVersion) {
	version, _ = u.d.GetRoomVersionForRoomNID(u.ctx, u.roomNID)
	return
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
	err := common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		for _, ref := range previousEventReferences {
			if err := u.d.statements.insertPreviousEvent(u.ctx, txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// IsReferenced implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (res bool, err error) {
	err = common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		err := u.d.statements.selectPreviousEventExists(u.ctx, txn, eventReference.EventID, eventReference.EventSHA256)
		if err == nil {
			res = true
			err = nil
		}
		if err == sql.ErrNoRows {
			res = false
			err = nil
		}
		return err
	})
	return
}

// SetLatestEvents implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) SetLatestEvents(
	roomNID types.RoomNID, latest []types.StateAtEventAndReference, lastEventNIDSent types.EventNID,
	currentStateSnapshotNID types.StateSnapshotNID,
) error {
	err := common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		eventNIDs := make([]types.EventNID, len(latest))
		for i := range latest {
			eventNIDs[i] = latest[i].EventNID
		}
		return u.d.statements.updateLatestEventNIDs(u.ctx, txn, roomNID, eventNIDs, lastEventNIDSent, currentStateSnapshotNID)
	})
	return err
}

// HasEventBeenSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) HasEventBeenSent(eventNID types.EventNID) (res bool, err error) {
	err = common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		res, err = u.d.statements.selectEventSentToOutput(u.ctx, txn, eventNID)
		return err
	})
	return
}

// MarkEventAsSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) MarkEventAsSent(eventNID types.EventNID) error {
	err := common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		return u.d.statements.updateEventSentToOutput(u.ctx, txn, eventNID)
	})
	return err
}

func (u *roomRecentEventsUpdater) MembershipUpdater(targetUserNID types.EventStateKeyNID) (mu types.MembershipUpdater, err error) {
	err = common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		mu, err = u.d.membershipUpdaterTxn(u.ctx, txn, u.roomNID, targetUserNID)
		return err
	})
	return
}

// RoomNID implements query.RoomserverQueryAPIDB
func (d *Database) RoomNID(ctx context.Context, roomID string) (roomNID types.RoomNID, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		roomNID, err = d.statements.selectRoomNID(ctx, txn, roomID)
		if err == sql.ErrNoRows {
			roomNID = 0
			err = nil
		}
		return err
	})
	return
}

// RoomNIDExcludingStubs implements query.RoomserverQueryAPIDB
func (d *Database) RoomNIDExcludingStubs(ctx context.Context, roomID string) (roomNID types.RoomNID, err error) {
	roomNID, err = d.RoomNID(ctx, roomID)
	if err != nil {
		return
	}
	latestEvents, _, err := d.statements.selectLatestEventNIDs(ctx, nil, roomNID)
	if err != nil {
		return
	}
	if len(latestEvents) == 0 {
		roomNID = 0
		return
	}
	return
}

// LatestEventIDs implements query.RoomserverQueryAPIDatabase
func (d *Database) LatestEventIDs(
	ctx context.Context, roomNID types.RoomNID,
) (references []gomatrixserverlib.EventReference, currentStateSnapshotNID types.StateSnapshotNID, depth int64, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		var eventNIDs []types.EventNID
		eventNIDs, currentStateSnapshotNID, err = d.statements.selectLatestEventNIDs(ctx, txn, roomNID)
		if err != nil {
			return err
		}
		references, err = d.statements.bulkSelectEventReference(ctx, txn, eventNIDs)
		if err != nil {
			return err
		}
		depth, err = d.statements.selectMaxEventDepth(ctx, txn, eventNIDs)
		if err != nil {
			return err
		}
		return nil
	})
	return
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
	return d.statements.insertRoomAlias(ctx, nil, alias, roomID, creatorUserID)
}

// GetRoomIDForAlias implements alias.RoomserverAliasAPIDB
func (d *Database) GetRoomIDForAlias(ctx context.Context, alias string) (string, error) {
	return d.statements.selectRoomIDFromAlias(ctx, nil, alias)
}

// GetAliasesForRoomID implements alias.RoomserverAliasAPIDB
func (d *Database) GetAliasesForRoomID(ctx context.Context, roomID string) ([]string, error) {
	return d.statements.selectAliasesFromRoomID(ctx, nil, roomID)
}

// GetCreatorIDForAlias implements alias.RoomserverAliasAPIDB
func (d *Database) GetCreatorIDForAlias(
	ctx context.Context, alias string,
) (string, error) {
	return d.statements.selectCreatorIDFromAlias(ctx, nil, alias)
}

// RemoveRoomAlias implements alias.RoomserverAliasAPIDB
func (d *Database) RemoveRoomAlias(ctx context.Context, alias string) error {
	return d.statements.deleteRoomAlias(ctx, nil, alias)
}

// StateEntriesForTuples implements state.RoomStateDatabase
func (d *Database) StateEntriesForTuples(
	ctx context.Context,
	stateBlockNIDs []types.StateBlockNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	return d.statements.bulkSelectFilteredStateBlockEntries(
		ctx, nil, stateBlockNIDs, stateKeyTuples,
	)
}

// MembershipUpdater implements input.RoomEventDatabase
func (d *Database) MembershipUpdater(
	ctx context.Context, roomID, targetUserID string,
	roomVersion gomatrixserverlib.RoomVersion,
) (updater types.MembershipUpdater, err error) {
	var txn *sql.Tx
	txn, err = d.db.Begin()
	if err != nil {
		return nil, err
	}
	succeeded := false
	defer func() {
		if !succeeded {
			txn.Rollback() // nolint: errcheck
		} else {
			// TODO: We should be holding open this transaction but we cannot have
			// multiple write transactions on sqlite. The code will perform additional
			// write transactions independent of this one which will consistently cause
			// 'database is locked' errors. For now, we'll break up the transaction and
			// hope we don't race too catastrophically. Long term, we should be able to
			// thread in txn objects where appropriate (either at the interface level or
			// bring matrix business logic into the storage layer).
			txerr := txn.Commit()
			if err == nil && txerr != nil {
				err = txerr
			}
		}
	}()

	roomNID, err := d.assignRoomNID(ctx, txn, roomID, roomVersion)
	if err != nil {
		return nil, err
	}

	targetUserNID, err := d.assignStateKeyNID(ctx, txn, targetUserID)
	if err != nil {
		return nil, err
	}

	updater, err = d.membershipUpdaterTxn(ctx, txn, roomNID, targetUserNID)
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
		// purposefully set the txn to nil so if we try to use it we panic and fail fast
		transaction{ctx, nil}, d, roomNID, targetUserNID, membership,
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
func (u *membershipUpdater) SetToInvite(event gomatrixserverlib.Event) (inserted bool, err error) {
	err = common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, txn, event.Sender())
		if err != nil {
			return err
		}
		inserted, err = u.d.statements.insertInviteEvent(
			u.ctx, txn, event.EventID(), u.roomNID, u.targetUserNID, senderUserNID, event.JSON(),
		)
		if err != nil {
			return err
		}
		if u.membership != membershipStateInvite {
			if err = u.d.statements.updateMembership(
				u.ctx, txn, u.roomNID, u.targetUserNID, senderUserNID, membershipStateInvite, 0,
			); err != nil {
				return err
			}
		}
		return nil
	})
	return
}

// SetToJoin implements types.MembershipUpdater
func (u *membershipUpdater) SetToJoin(senderUserID string, eventID string, isUpdate bool) (inviteEventIDs []string, err error) {
	err = common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, txn, senderUserID)
		if err != nil {
			return err
		}

		// If this is a join event update, there is no invite to update
		if !isUpdate {
			inviteEventIDs, err = u.d.statements.updateInviteRetired(
				u.ctx, txn, u.roomNID, u.targetUserNID,
			)
			if err != nil {
				return err
			}
		}

		// Look up the NID of the new join event
		nIDs, err := u.d.EventNIDs(u.ctx, []string{eventID})
		if err != nil {
			return err
		}

		if u.membership != membershipStateJoin || isUpdate {
			if err = u.d.statements.updateMembership(
				u.ctx, txn, u.roomNID, u.targetUserNID, senderUserNID,
				membershipStateJoin, nIDs[eventID],
			); err != nil {
				return err
			}
		}
		return nil
	})

	return
}

// SetToLeave implements types.MembershipUpdater
func (u *membershipUpdater) SetToLeave(senderUserID string, eventID string) (inviteEventIDs []string, err error) {
	err = common.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, txn, senderUserID)
		if err != nil {
			return err
		}
		inviteEventIDs, err = u.d.statements.updateInviteRetired(
			u.ctx, txn, u.roomNID, u.targetUserNID,
		)
		if err != nil {
			return err
		}

		// Look up the NID of the new leave event
		nIDs, err := u.d.EventNIDs(u.ctx, []string{eventID})
		if err != nil {
			return err
		}

		if u.membership != membershipStateLeaveOrBan {
			if err = u.d.statements.updateMembership(
				u.ctx, txn, u.roomNID, u.targetUserNID, senderUserNID,
				membershipStateLeaveOrBan, nIDs[eventID],
			); err != nil {
				return err
			}
		}
		return nil
	})
	return
}

// GetMembership implements query.RoomserverQueryAPIDB
func (d *Database) GetMembership(
	ctx context.Context, roomNID types.RoomNID, requestSenderUserID string,
) (membershipEventNID types.EventNID, stillInRoom bool, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		requestSenderUserNID, err := d.assignStateKeyNID(ctx, txn, requestSenderUserID)
		if err != nil {
			return err
		}

		membershipEventNID, _, err =
			d.statements.selectMembershipFromRoomAndTarget(
				ctx, txn, roomNID, requestSenderUserNID,
			)
		if err == sql.ErrNoRows {
			// The user has never been a member of that room
			return nil
		}
		if err != nil {
			return err
		}
		stillInRoom = true
		return nil
	})

	return
}

// GetMembershipEventNIDsForRoom implements query.RoomserverQueryAPIDB
func (d *Database) GetMembershipEventNIDsForRoom(
	ctx context.Context, roomNID types.RoomNID, joinOnly bool,
) (eventNIDs []types.EventNID, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		if joinOnly {
			eventNIDs, err = d.statements.selectMembershipsFromRoomAndMembership(
				ctx, txn, roomNID, membershipStateJoin,
			)
			return nil
		}

		eventNIDs, err = d.statements.selectMembershipsFromRoom(ctx, txn, roomNID)
		return nil
	})
	return
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

func (d *Database) GetRoomVersionForRoom(
	ctx context.Context, roomID string,
) (gomatrixserverlib.RoomVersion, error) {
	return d.statements.selectRoomVersionForRoomID(
		ctx, nil, roomID,
	)
}

func (d *Database) GetRoomVersionForRoomNID(
	ctx context.Context, roomNID types.RoomNID,
) (gomatrixserverlib.RoomVersion, error) {
	return d.statements.selectRoomVersionForRoomNID(
		ctx, nil, roomNID,
	)
}

type transaction struct {
	ctx context.Context
	txn *sql.Tx
}

// Commit implements types.Transaction
func (t *transaction) Commit() error {
	if t.txn == nil {
		return nil
	}
	return t.txn.Commit()
}

// Rollback implements types.Transaction
func (t *transaction) Rollback() error {
	if t.txn == nil {
		return nil
	}
	return t.txn.Rollback()
}
