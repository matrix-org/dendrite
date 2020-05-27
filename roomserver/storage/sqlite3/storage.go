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
	"errors"
	"net/url"

	"github.com/matrix-org/dendrite/internal/sqlutil"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	_ "github.com/mattn/go-sqlite3"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
	statements     statements
	events         tables.Events
	eventJSON      tables.EventJSON
	eventTypes     tables.EventTypes
	eventStateKeys tables.EventStateKeys
	rooms          tables.Rooms
	transactions   tables.Transactions
	prevEvents     tables.PreviousEvents
	invites        tables.Invites
	db             *sql.DB
}

// Open a sqlite database.
// nolint: gocyclo
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
	if d.db, err = sqlutil.Open(internal.SQLiteDriverName(), cs, nil); err != nil {
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
	d.eventStateKeys, err = NewSqliteEventStateKeysTable(d.db)
	if err != nil {
		return nil, err
	}
	d.eventTypes, err = NewSqliteEventTypesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.eventJSON, err = NewSqliteEventJSONTable(d.db)
	if err != nil {
		return nil, err
	}
	d.events, err = NewSqliteEventsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.rooms, err = NewSqliteRoomsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.transactions, err = NewSqliteTransactionsTable(d.db)
	if err != nil {
		return nil, err
	}
	stateBlock, err := NewSqliteStateBlockTable(d.db)
	if err != nil {
		return nil, err
	}
	stateSnapshot, err := NewSqliteStateSnapshotTable(d.db)
	if err != nil {
		return nil, err
	}
	d.prevEvents, err = NewSqlitePrevEventsTable(d.db)
	if err != nil {
		return nil, err
	}
	roomAliases, err := NewSqliteRoomAliasesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.invites, err = NewSqliteInvitesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                  d.db,
		EventsTable:         d.events,
		EventTypesTable:     d.eventTypes,
		EventStateKeysTable: d.eventStateKeys,
		EventJSONTable:      d.eventJSON,
		RoomsTable:          d.rooms,
		TransactionsTable:   d.transactions,
		StateBlockTable:     stateBlock,
		StateSnapshotTable:  stateSnapshot,
		PrevEventsTable:     d.prevEvents,
		RoomAliasesTable:    roomAliases,
		InvitesTable:        d.invites,
	}
	return &d, nil
}

func (d *Database) assignRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (roomNID types.RoomNID, err error) {
	// Check if we already have a numeric ID in the database.
	roomNID, err = d.rooms.SelectRoomNID(ctx, txn, roomID)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		roomNID, err = d.rooms.InsertRoomNID(ctx, txn, roomID, roomVersion)
		if err == nil {
			// Now get the numeric ID back out of the database
			roomNID, err = d.rooms.SelectRoomNID(ctx, txn, roomID)
		}
	}
	return
}

func (d *Database) assignStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (eventStateKeyNID types.EventStateKeyNID, err error) {
	// Check if we already have a numeric ID in the database.
	eventStateKeyNID, err = d.eventStateKeys.SelectEventStateKeyNID(ctx, txn, eventStateKey)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventStateKeyNID, err = d.eventStateKeys.InsertEventStateKeyNID(ctx, txn, eventStateKey)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventStateKeyNID, err = d.eventStateKeys.SelectEventStateKeyNID(ctx, txn, eventStateKey)
		}
	}
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
		d.rooms.SelectLatestEventsNIDsForUpdate(ctx, txn, roomNID)
	if err != nil {
		txn.Rollback() // nolint: errcheck
		return nil, err
	}
	stateAndRefs, err := d.events.BulkSelectStateAtEventAndReference(ctx, txn, eventNIDs)
	if err != nil {
		txn.Rollback() // nolint: errcheck
		return nil, err
	}
	var lastEventIDSent string
	if lastEventNIDSent != 0 {
		lastEventIDSent, err = d.events.SelectEventID(ctx, txn, lastEventNIDSent)
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
	err := internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		for _, ref := range previousEventReferences {
			if err := u.d.prevEvents.InsertPreviousEvent(u.ctx, txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// IsReferenced implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (res bool, err error) {
	err = internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		err := u.d.prevEvents.SelectPreviousEventExists(u.ctx, txn, eventReference.EventID, eventReference.EventSHA256)
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
	err := internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		eventNIDs := make([]types.EventNID, len(latest))
		for i := range latest {
			eventNIDs[i] = latest[i].EventNID
		}
		return u.d.rooms.UpdateLatestEventNIDs(u.ctx, txn, roomNID, eventNIDs, lastEventNIDSent, currentStateSnapshotNID)
	})
	return err
}

// HasEventBeenSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) HasEventBeenSent(eventNID types.EventNID) (res bool, err error) {
	err = internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		res, err = u.d.events.SelectEventSentToOutput(u.ctx, txn, eventNID)
		return err
	})
	return
}

// MarkEventAsSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) MarkEventAsSent(eventNID types.EventNID) error {
	err := internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		return u.d.events.UpdateEventSentToOutput(u.ctx, txn, eventNID)
	})
	return err
}

func (u *roomRecentEventsUpdater) MembershipUpdater(targetUserNID types.EventStateKeyNID, targetLocal bool) (mu types.MembershipUpdater, err error) {
	err = internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		mu, err = u.d.membershipUpdaterTxn(u.ctx, txn, u.roomNID, targetUserNID, targetLocal)
		return err
	})
	return
}

// MembershipUpdater implements input.RoomEventDatabase
func (d *Database) MembershipUpdater(
	ctx context.Context, roomID, targetUserID string,
	targetLocal bool, roomVersion gomatrixserverlib.RoomVersion,
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

	updater, err = d.membershipUpdaterTxn(ctx, txn, roomNID, targetUserNID, targetLocal)
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
	targetLocal bool,
) (types.MembershipUpdater, error) {

	if err := d.statements.insertMembership(ctx, txn, roomNID, targetUserNID, targetLocal); err != nil {
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
	err = internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, txn, event.Sender())
		if err != nil {
			return err
		}
		inserted, err = u.d.invites.InsertInviteEvent(
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
	err = internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, txn, senderUserID)
		if err != nil {
			return err
		}

		// If this is a join event update, there is no invite to update
		if !isUpdate {
			inviteEventIDs, err = u.d.invites.UpdateInviteRetired(
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
	err = internal.WithTransaction(u.d.db, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, txn, senderUserID)
		if err != nil {
			return err
		}
		inviteEventIDs, err = u.d.invites.UpdateInviteRetired(
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
	err = internal.WithTransaction(d.db, func(txn *sql.Tx) error {
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
	ctx context.Context, roomNID types.RoomNID, joinOnly bool, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	err = internal.WithTransaction(d.db, func(txn *sql.Tx) error {
		if joinOnly {
			eventNIDs, err = d.statements.selectMembershipsFromRoomAndMembership(
				ctx, txn, roomNID, membershipStateJoin, localOnly,
			)
			return nil
		}

		eventNIDs, err = d.statements.selectMembershipsFromRoom(ctx, txn, roomNID, localOnly)
		return nil
	})
	return
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
