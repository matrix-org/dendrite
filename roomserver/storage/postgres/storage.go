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

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"

	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// A Database is used to store room events and stream offsets.
type Database struct {
	shared.Database
	events         tables.Events
	eventTypes     tables.EventTypes
	eventStateKeys tables.EventStateKeys
	eventJSON      tables.EventJSON
	rooms          tables.Rooms
	transactions   tables.Transactions
	prevEvents     tables.PreviousEvents
	invites        tables.Invites
	membership     tables.Membership
	db             *sql.DB
}

// Open a postgres database.
// nolint: gocyclo
func Open(dataSourceName string, dbProperties internal.DbProperties) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sqlutil.Open("postgres", dataSourceName, dbProperties); err != nil {
		return nil, err
	}
	d.eventStateKeys, err = NewPostgresEventStateKeysTable(d.db)
	if err != nil {
		return nil, err
	}
	d.eventTypes, err = NewPostgresEventTypesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.eventJSON, err = NewPostgresEventJSONTable(d.db)
	if err != nil {
		return nil, err
	}
	d.events, err = NewPostgresEventsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.rooms, err = NewPostgresRoomsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.transactions, err = NewPostgresTransactionsTable(d.db)
	if err != nil {
		return nil, err
	}
	stateBlock, err := NewPostgresStateBlockTable(d.db)
	if err != nil {
		return nil, err
	}
	stateSnapshot, err := NewPostgresStateSnapshotTable(d.db)
	if err != nil {
		return nil, err
	}
	roomAliases, err := NewPostgresRoomAliasesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.prevEvents, err = NewPostgresPreviousEventsTable(d.db)
	if err != nil {
		return nil, err
	}
	d.invites, err = NewPostgresInvitesTable(d.db)
	if err != nil {
		return nil, err
	}
	d.membership, err = NewPostgresMembershipTable(d.db)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                  d.db,
		EventTypesTable:     d.eventTypes,
		EventStateKeysTable: d.eventStateKeys,
		EventJSONTable:      d.eventJSON,
		EventsTable:         d.events,
		RoomsTable:          d.rooms,
		TransactionsTable:   d.transactions,
		StateBlockTable:     stateBlock,
		StateSnapshotTable:  stateSnapshot,
		PrevEventsTable:     d.prevEvents,
		RoomAliasesTable:    roomAliases,
		InvitesTable:        d.invites,
		MembershipTable:     d.membership,
	}
	return &d, nil
}

func (d *Database) assignRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (types.RoomNID, error) {
	// Check if we already have a numeric ID in the database.
	roomNID, err := d.rooms.SelectRoomNID(ctx, txn, roomID)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		roomNID, err = d.rooms.InsertRoomNID(ctx, txn, roomID, roomVersion)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			roomNID, err = d.rooms.SelectRoomNID(ctx, txn, roomID)
		}
	}
	return roomNID, err
}

func (d *Database) assignStateKeyNID(
	ctx context.Context, txn *sql.Tx, eventStateKey string,
) (types.EventStateKeyNID, error) {
	// Check if we already have a numeric ID in the database.
	eventStateKeyNID, err := d.eventStateKeys.SelectEventStateKeyNID(ctx, txn, eventStateKey)
	if err == sql.ErrNoRows {
		// We don't have a numeric ID so insert one into the database.
		eventStateKeyNID, err = d.eventStateKeys.InsertEventStateKeyNID(ctx, txn, eventStateKey)
		if err == sql.ErrNoRows {
			// We raced with another insert so run the select again.
			eventStateKeyNID, err = d.eventStateKeys.SelectEventStateKeyNID(ctx, txn, eventStateKey)
		}
	}
	return eventStateKeyNID, err
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
	return &roomRecentEventsUpdater{
		transaction{ctx, txn}, d, roomNID, stateAndRefs, lastEventIDSent, currentStateSnapshotNID,
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
	for _, ref := range previousEventReferences {
		if err := u.d.prevEvents.InsertPreviousEvent(u.ctx, u.txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
			return err
		}
	}
	return nil
}

// IsReferenced implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (bool, error) {
	err := u.d.prevEvents.SelectPreviousEventExists(u.ctx, u.txn, eventReference.EventID, eventReference.EventSHA256)
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
	return u.d.rooms.UpdateLatestEventNIDs(u.ctx, u.txn, roomNID, eventNIDs, lastEventNIDSent, currentStateSnapshotNID)
}

// HasEventBeenSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) HasEventBeenSent(eventNID types.EventNID) (bool, error) {
	return u.d.events.SelectEventSentToOutput(u.ctx, u.txn, eventNID)
}

// MarkEventAsSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) MarkEventAsSent(eventNID types.EventNID) error {
	return u.d.events.UpdateEventSentToOutput(u.ctx, u.txn, eventNID)
}

func (u *roomRecentEventsUpdater) MembershipUpdater(targetUserNID types.EventStateKeyNID, targetLocal bool) (types.MembershipUpdater, error) {
	return u.d.membershipUpdaterTxn(u.ctx, u.txn, u.roomNID, targetUserNID, targetLocal)
}

// MembershipUpdater implements input.RoomEventDatabase
func (d *Database) MembershipUpdater(
	ctx context.Context, roomID, targetUserID string,
	targetLocal bool, roomVersion gomatrixserverlib.RoomVersion,
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

	roomNID, err := d.assignRoomNID(ctx, txn, roomID, roomVersion)
	if err != nil {
		return nil, err
	}

	targetUserNID, err := d.assignStateKeyNID(ctx, txn, targetUserID)
	if err != nil {
		return nil, err
	}

	updater, err := d.membershipUpdaterTxn(ctx, txn, roomNID, targetUserNID, targetLocal)
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
	membership    tables.MembershipState
}

func (d *Database) membershipUpdaterTxn(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	targetUserNID types.EventStateKeyNID,
	targetLocal bool,
) (types.MembershipUpdater, error) {

	if err := d.membership.InsertMembership(ctx, txn, roomNID, targetUserNID, targetLocal); err != nil {
		return nil, err
	}

	membership, err := d.membership.SelectMembershipForUpdate(ctx, txn, roomNID, targetUserNID)
	if err != nil {
		return nil, err
	}

	return &membershipUpdater{
		transaction{ctx, txn}, d, roomNID, targetUserNID, membership,
	}, nil
}

// IsInvite implements types.MembershipUpdater
func (u *membershipUpdater) IsInvite() bool {
	return u.membership == tables.MembershipStateInvite
}

// IsJoin implements types.MembershipUpdater
func (u *membershipUpdater) IsJoin() bool {
	return u.membership == tables.MembershipStateJoin
}

// IsLeave implements types.MembershipUpdater
func (u *membershipUpdater) IsLeave() bool {
	return u.membership == tables.MembershipStateLeaveOrBan
}

// SetToInvite implements types.MembershipUpdater
func (u *membershipUpdater) SetToInvite(event gomatrixserverlib.Event) (bool, error) {
	senderUserNID, err := u.d.assignStateKeyNID(u.ctx, u.txn, event.Sender())
	if err != nil {
		return false, err
	}
	inserted, err := u.d.invites.InsertInviteEvent(
		u.ctx, u.txn, event.EventID(), u.roomNID, u.targetUserNID, senderUserNID, event.JSON(),
	)
	if err != nil {
		return false, err
	}
	if u.membership != tables.MembershipStateInvite {
		if err = u.d.membership.UpdateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID, tables.MembershipStateInvite, 0,
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
		inviteEventIDs, err = u.d.invites.UpdateInviteRetired(
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

	if u.membership != tables.MembershipStateJoin || isUpdate {
		if err = u.d.membership.UpdateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID,
			tables.MembershipStateJoin, nIDs[eventID],
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
	inviteEventIDs, err := u.d.invites.UpdateInviteRetired(
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

	if u.membership != tables.MembershipStateLeaveOrBan {
		if err = u.d.membership.UpdateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID,
			tables.MembershipStateLeaveOrBan, nIDs[eventID],
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
		d.membership.SelectMembershipFromRoomAndTarget(
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

// GetMembershipEventNIDsForRoom implements query.RoomserverQueryAPIDB
func (d *Database) GetMembershipEventNIDsForRoom(
	ctx context.Context, roomNID types.RoomNID, joinOnly bool, localOnly bool,
) ([]types.EventNID, error) {
	if joinOnly {
		return d.membership.SelectMembershipsFromRoomAndMembership(
			ctx, roomNID, tables.MembershipStateJoin, localOnly,
		)
	}

	return d.membership.SelectMembershipsFromRoom(ctx, roomNID, localOnly)
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
