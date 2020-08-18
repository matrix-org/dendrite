package shared

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type roomRecentEventsUpdater struct {
	transaction
	d                       *Database
	roomNID                 types.RoomNID
	latestEvents            []types.StateAtEventAndReference
	lastEventIDSent         string
	currentStateSnapshotNID types.StateSnapshotNID
}

func NewRoomRecentEventsUpdater(d *Database, ctx context.Context, roomNID types.RoomNID, useTxns bool) (types.RoomRecentEventsUpdater, func() error, error) {
	eventNIDs, lastEventNIDSent, currentStateSnapshotNID, err :=
		d.RoomsTable.SelectLatestEventsNIDsForUpdate(ctx, nil, roomNID)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, fmt.Errorf("d.RoomsTable.SelectLatestEventsNIDsForUpdate: %w", err)
	}
	var stateAndRefs []types.StateAtEventAndReference
	var lastEventIDSent string
	if err == nil {
		stateAndRefs, err = d.EventsTable.BulkSelectStateAtEventAndReference(ctx, nil, eventNIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("d.EventsTable.BulkSelectStateAtEventAndReference: %w", err)
		}
		if lastEventNIDSent != 0 {
			lastEventIDSent, err = d.EventsTable.SelectEventID(ctx, nil, lastEventNIDSent)
			if err != nil {
				return nil, nil, fmt.Errorf("d.EventsTable.SelectEventID: %w", err)
			}
		}
	}
	var txn *sql.Tx
	cancel := func() error { return nil }
	if useTxns {
		txn, err = d.DB.Begin()
		if err != nil {
			return nil, nil, fmt.Errorf("d.DB.Begin: %w", err)
		}
		cancel = func() error { return txn.Commit() }
	}
	return &roomRecentEventsUpdater{
		transaction{ctx, txn}, d, roomNID, stateAndRefs, lastEventIDSent, currentStateSnapshotNID,
	}, cancel, nil
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
	return u.d.Writer.Do(u.d.DB, nil, func(txn *sql.Tx) error {
		for _, ref := range previousEventReferences {
			if err := u.d.PrevEventsTable.InsertPreviousEvent(u.ctx, u.txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
				return fmt.Errorf("u.d.PrevEventsTable.InsertPreviousEvent: %w", err)
			}
		}
		return nil
	})
}

// IsReferenced implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (bool, error) {
	err := u.d.PrevEventsTable.SelectPreviousEventExists(u.ctx, u.txn, eventReference.EventID, eventReference.EventSHA256)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, fmt.Errorf("u.d.PrevEventsTable.SelectPreviousEventExists: %w", err)
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
	return u.d.Writer.Do(u.d.DB, nil, func(txn *sql.Tx) error {
		if err := u.d.RoomsTable.UpdateLatestEventNIDs(u.ctx, u.txn, roomNID, eventNIDs, lastEventNIDSent, currentStateSnapshotNID); err != nil {
			return fmt.Errorf("u.d.RoomsTable.updateLatestEventNIDs: %w", err)
		}
		return nil
	})
}

// HasEventBeenSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) HasEventBeenSent(eventNID types.EventNID) (bool, error) {
	return u.d.EventsTable.SelectEventSentToOutput(u.ctx, u.txn, eventNID)
}

// MarkEventAsSent implements types.RoomRecentEventsUpdater
func (u *roomRecentEventsUpdater) MarkEventAsSent(eventNID types.EventNID) error {
	return u.d.Writer.Do(u.d.DB, nil, func(txn *sql.Tx) error {
		return u.d.EventsTable.UpdateEventSentToOutput(u.ctx, u.txn, eventNID)
	})
}

func (u *roomRecentEventsUpdater) MembershipUpdater(targetUserNID types.EventStateKeyNID, targetLocal bool) (types.MembershipUpdater, func() error, error) {
	return u.d.membershipUpdaterTxn(u.ctx, u.txn, u.roomNID, targetUserNID, targetLocal)
}
