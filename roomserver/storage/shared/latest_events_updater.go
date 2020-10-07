package shared

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type LatestEventsUpdater struct {
	transaction
	d                       *Database
	roomInfo                types.RoomInfo
	latestEvents            []types.StateAtEventAndReference
	lastEventIDSent         string
	currentStateSnapshotNID types.StateSnapshotNID
}

func NewLatestEventsUpdater(ctx context.Context, d *Database, txn *sql.Tx, roomInfo types.RoomInfo) (*LatestEventsUpdater, error) {
	eventNIDs, lastEventNIDSent, currentStateSnapshotNID, err :=
		d.RoomsTable.SelectLatestEventsNIDsForUpdate(ctx, txn, roomInfo.RoomNID)
	if err != nil {
		txn.Rollback() // nolint: errcheck
		return nil, err
	}
	stateAndRefs, err := d.EventsTable.BulkSelectStateAtEventAndReference(ctx, txn, eventNIDs)
	if err != nil {
		txn.Rollback() // nolint: errcheck
		return nil, err
	}
	var lastEventIDSent string
	if lastEventNIDSent != 0 {
		lastEventIDSent, err = d.EventsTable.SelectEventID(ctx, txn, lastEventNIDSent)
		if err != nil {
			txn.Rollback() // nolint: errcheck
			return nil, err
		}
	}
	return &LatestEventsUpdater{
		transaction{ctx, txn}, d, roomInfo, stateAndRefs, lastEventIDSent, currentStateSnapshotNID,
	}, nil
}

// RoomVersion implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) RoomVersion() (version gomatrixserverlib.RoomVersion) {
	return u.roomInfo.RoomVersion
}

// LatestEvents implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) LatestEvents() []types.StateAtEventAndReference {
	return u.latestEvents
}

// LastEventIDSent implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) LastEventIDSent() string {
	return u.lastEventIDSent
}

// CurrentStateSnapshotNID implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) CurrentStateSnapshotNID() types.StateSnapshotNID {
	return u.currentStateSnapshotNID
}

// StorePreviousEvents implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) StorePreviousEvents(eventNID types.EventNID, previousEventReferences []gomatrixserverlib.EventReference) error {
	return u.d.Writer.Do(u.d.DB, u.txn, func(txn *sql.Tx) error {
		for _, ref := range previousEventReferences {
			if err := u.d.PrevEventsTable.InsertPreviousEvent(u.ctx, txn, ref.EventID, ref.EventSHA256, eventNID); err != nil {
				return fmt.Errorf("u.d.PrevEventsTable.InsertPreviousEvent: %w", err)
			}
		}
		return nil
	})
}

// IsReferenced implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) IsReferenced(eventReference gomatrixserverlib.EventReference) (bool, error) {
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
func (u *LatestEventsUpdater) SetLatestEvents(
	roomNID types.RoomNID, latest []types.StateAtEventAndReference, lastEventNIDSent types.EventNID,
	currentStateSnapshotNID types.StateSnapshotNID,
) error {
	eventNIDs := make([]types.EventNID, len(latest))
	for i := range latest {
		eventNIDs[i] = latest[i].EventNID
	}
	return u.d.Writer.Do(u.d.DB, u.txn, func(txn *sql.Tx) error {
		if err := u.d.RoomsTable.UpdateLatestEventNIDs(u.ctx, txn, roomNID, eventNIDs, lastEventNIDSent, currentStateSnapshotNID); err != nil {
			return fmt.Errorf("u.d.RoomsTable.updateLatestEventNIDs: %w", err)
		}
		return nil
	})
}

// HasEventBeenSent implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) HasEventBeenSent(eventNID types.EventNID) (bool, error) {
	return u.d.EventsTable.SelectEventSentToOutput(u.ctx, u.txn, eventNID)
}

// MarkEventAsSent implements types.RoomRecentEventsUpdater
func (u *LatestEventsUpdater) MarkEventAsSent(eventNID types.EventNID) error {
	return u.d.Writer.Do(u.d.DB, u.txn, func(txn *sql.Tx) error {
		return u.d.EventsTable.UpdateEventSentToOutput(u.ctx, txn, eventNID)
	})
}

func (u *LatestEventsUpdater) MembershipUpdater(targetUserNID types.EventStateKeyNID, targetLocal bool) (*MembershipUpdater, error) {
	return u.d.membershipUpdaterTxn(u.ctx, u.txn, u.roomInfo.RoomNID, targetUserNID, targetLocal)
}
