package shared

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type MembershipUpdater struct {
	transaction
	d             *Database
	roomNID       types.RoomNID
	targetUserNID types.EventStateKeyNID
	membership    tables.MembershipState
}

func NewMembershipUpdater(
	ctx context.Context, d *Database, txn *sql.Tx, roomID, targetUserID string,
	targetLocal bool, roomVersion gomatrixserverlib.RoomVersion,
) (*MembershipUpdater, error) {
	var roomNID types.RoomNID
	var targetUserNID types.EventStateKeyNID
	var err error
	err = d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
		roomNID, err = d.assignRoomNID(ctx, roomID, roomVersion)
		if err != nil {
			return err
		}

		targetUserNID, err = d.assignStateKeyNID(ctx, targetUserID)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return d.membershipUpdaterTxn(ctx, txn, roomNID, targetUserNID, targetLocal)
}

func (d *Database) membershipUpdaterTxn(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	targetUserNID types.EventStateKeyNID,
	targetLocal bool,
) (*MembershipUpdater, error) {
	err := d.Writer.Do(d.DB, txn, func(txn *sql.Tx) error {
		if err := d.MembershipTable.InsertMembership(ctx, txn, roomNID, targetUserNID, targetLocal); err != nil {
			return fmt.Errorf("d.MembershipTable.InsertMembership: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("u.d.Writer.Do: %w", err)
	}

	membership, err := d.MembershipTable.SelectMembershipForUpdate(ctx, txn, roomNID, targetUserNID)
	if err != nil {
		return nil, err
	}

	return &MembershipUpdater{
		transaction{ctx, txn}, d, roomNID, targetUserNID, membership,
	}, nil
}

// IsInvite implements types.MembershipUpdater
func (u *MembershipUpdater) IsInvite() bool {
	return u.membership == tables.MembershipStateInvite
}

// IsJoin implements types.MembershipUpdater
func (u *MembershipUpdater) IsJoin() bool {
	return u.membership == tables.MembershipStateJoin
}

// IsLeave implements types.MembershipUpdater
func (u *MembershipUpdater) IsLeave() bool {
	return u.membership == tables.MembershipStateLeaveOrBan
}

// IsKnock implements types.MembershipUpdater
func (u *MembershipUpdater) IsKnock() bool {
	return u.membership == tables.MembershipStateKnock
}

// SetToInvite implements types.MembershipUpdater
func (u *MembershipUpdater) SetToInvite(event *gomatrixserverlib.Event) (bool, error) {
	var inserted bool
	err := u.d.Writer.Do(u.d.DB, u.txn, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, event.Sender())
		if err != nil {
			return fmt.Errorf("u.d.AssignStateKeyNID: %w", err)
		}
		inserted, err = u.d.InvitesTable.InsertInviteEvent(
			u.ctx, u.txn, event.EventID(), u.roomNID, u.targetUserNID, senderUserNID, event.JSON(),
		)
		if err != nil {
			return fmt.Errorf("u.d.InvitesTable.InsertInviteEvent: %w", err)
		}
		if u.membership != tables.MembershipStateInvite {
			if inserted, err = u.d.MembershipTable.UpdateMembership(u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID, tables.MembershipStateInvite, 0, false); err != nil {
				return fmt.Errorf("u.d.MembershipTable.UpdateMembership: %w", err)
			}
		}
		return nil
	})
	return inserted, err
}

// SetToJoin implements types.MembershipUpdater
func (u *MembershipUpdater) SetToJoin(senderUserID string, eventID string, isUpdate bool) ([]string, error) {
	var inviteEventIDs []string

	err := u.d.Writer.Do(u.d.DB, u.txn, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, senderUserID)
		if err != nil {
			return fmt.Errorf("u.d.AssignStateKeyNID: %w", err)
		}

		// If this is a join event update, there is no invite to update
		if !isUpdate {
			inviteEventIDs, err = u.d.InvitesTable.UpdateInviteRetired(
				u.ctx, u.txn, u.roomNID, u.targetUserNID,
			)
			if err != nil {
				return fmt.Errorf("u.d.InvitesTables.UpdateInviteRetired: %w", err)
			}
		}

		// Look up the NID of the new join event
		nIDs, err := u.d.eventNIDs(u.ctx, u.txn, []string{eventID}, false)
		if err != nil {
			return fmt.Errorf("u.d.EventNIDs: %w", err)
		}

		if u.membership != tables.MembershipStateJoin || isUpdate {
			if _, err = u.d.MembershipTable.UpdateMembership(u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID, tables.MembershipStateJoin, nIDs[eventID], false); err != nil {
				return fmt.Errorf("u.d.MembershipTable.UpdateMembership: %w", err)
			}
		}

		return nil
	})

	return inviteEventIDs, err
}

// SetToLeave implements types.MembershipUpdater
func (u *MembershipUpdater) SetToLeave(senderUserID string, eventID string) ([]string, error) {
	var inviteEventIDs []string

	err := u.d.Writer.Do(u.d.DB, u.txn, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, senderUserID)
		if err != nil {
			return fmt.Errorf("u.d.AssignStateKeyNID: %w", err)
		}
		inviteEventIDs, err = u.d.InvitesTable.UpdateInviteRetired(
			u.ctx, u.txn, u.roomNID, u.targetUserNID,
		)
		if err != nil {
			return fmt.Errorf("u.d.InvitesTable.updateInviteRetired: %w", err)
		}

		// Look up the NID of the new leave event
		nIDs, err := u.d.eventNIDs(u.ctx, u.txn, []string{eventID}, false)
		if err != nil {
			return fmt.Errorf("u.d.EventNIDs: %w", err)
		}

		if u.membership != tables.MembershipStateLeaveOrBan {
			if _, err = u.d.MembershipTable.UpdateMembership(u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID, tables.MembershipStateLeaveOrBan, nIDs[eventID], false); err != nil {
				return fmt.Errorf("u.d.MembershipTable.UpdateMembership: %w", err)
			}
		}

		return nil
	})
	return inviteEventIDs, err
}

// SetToKnock implements types.MembershipUpdater
func (u *MembershipUpdater) SetToKnock(event *gomatrixserverlib.Event) (bool, error) {
	var inserted bool
	err := u.d.Writer.Do(u.d.DB, u.txn, func(txn *sql.Tx) error {
		senderUserNID, err := u.d.assignStateKeyNID(u.ctx, event.Sender())
		if err != nil {
			return fmt.Errorf("u.d.AssignStateKeyNID: %w", err)
		}
		if u.membership != tables.MembershipStateKnock {
			// Look up the NID of the new knock event
			nIDs, err := u.d.eventNIDs(u.ctx, u.txn, []string{event.EventID()}, false)
			if err != nil {
				return fmt.Errorf("u.d.EventNIDs: %w", err)
			}

			if inserted, err = u.d.MembershipTable.UpdateMembership(u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID, tables.MembershipStateKnock, nIDs[event.EventID()], false); err != nil {
				return fmt.Errorf("u.d.MembershipTable.UpdateMembership: %w", err)
			}
		}
		return nil
	})
	return inserted, err
}
