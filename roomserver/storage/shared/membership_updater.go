package shared

import (
	"context"
	"database/sql"

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
	roomNID, err := d.assignRoomNID(ctx, txn, roomID, roomVersion)
	if err != nil {
		return nil, err
	}

	targetUserNID, err := d.assignStateKeyNID(ctx, txn, targetUserID)
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

	if err := d.MembershipTable.InsertMembership(ctx, txn, roomNID, targetUserNID, targetLocal); err != nil {
		return nil, err
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

// SetToInvite implements types.MembershipUpdater
func (u *MembershipUpdater) SetToInvite(event gomatrixserverlib.Event) (bool, error) {
	senderUserNID, err := u.d.assignStateKeyNID(u.ctx, u.txn, event.Sender())
	if err != nil {
		return false, err
	}
	inserted, err := u.d.InvitesTable.InsertInviteEvent(
		u.ctx, u.txn, event.EventID(), u.roomNID, u.targetUserNID, senderUserNID, event.JSON(),
	)
	if err != nil {
		return false, err
	}
	if u.membership != tables.MembershipStateInvite {
		if err = u.d.MembershipTable.UpdateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID, tables.MembershipStateInvite, 0,
		); err != nil {
			return false, err
		}
	}
	return inserted, nil
}

// SetToJoin implements types.MembershipUpdater
func (u *MembershipUpdater) SetToJoin(senderUserID string, eventID string, isUpdate bool) ([]string, error) {
	var inviteEventIDs []string

	senderUserNID, err := u.d.assignStateKeyNID(u.ctx, u.txn, senderUserID)
	if err != nil {
		return nil, err
	}

	// If this is a join event update, there is no invite to update
	if !isUpdate {
		inviteEventIDs, err = u.d.InvitesTable.UpdateInviteRetired(
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
		if err = u.d.MembershipTable.UpdateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID,
			tables.MembershipStateJoin, nIDs[eventID],
		); err != nil {
			return nil, err
		}
	}

	return inviteEventIDs, nil
}

// SetToLeave implements types.MembershipUpdater
func (u *MembershipUpdater) SetToLeave(senderUserID string, eventID string) ([]string, error) {
	senderUserNID, err := u.d.assignStateKeyNID(u.ctx, u.txn, senderUserID)
	if err != nil {
		return nil, err
	}
	inviteEventIDs, err := u.d.InvitesTable.UpdateInviteRetired(
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
		if err = u.d.MembershipTable.UpdateMembership(
			u.ctx, u.txn, u.roomNID, u.targetUserNID, senderUserNID,
			tables.MembershipStateLeaveOrBan, nIDs[eventID],
		); err != nil {
			return nil, err
		}
	}
	return inviteEventIDs, nil
}
