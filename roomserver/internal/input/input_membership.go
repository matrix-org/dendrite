// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package input

import (
	"context"
	"fmt"

	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/internal/helpers"
	"github.com/element-hq/dendrite/roomserver/storage/shared"
	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/element-hq/dendrite/roomserver/types"
)

// updateMembership updates the current membership and the invites for each
// user affected by a change in the current state of the room.
// Returns a list of output events to write to the kafka log to inform the
// consumers about the invites added or retired by the change in current state.
func (r *Inputer) updateMemberships(
	ctx context.Context,
	updater *shared.RoomUpdater,
	removed, added []types.StateEntry,
) ([]api.OutputEvent, error) {
	trace, ctx := internal.StartRegion(ctx, "updateMemberships")
	defer trace.EndRegion()

	changes := membershipChanges(removed, added)
	var eventNIDs []types.EventNID
	for _, change := range changes {
		if change.addedEventNID != 0 {
			eventNIDs = append(eventNIDs, change.addedEventNID)
		}
		if change.removedEventNID != 0 {
			eventNIDs = append(eventNIDs, change.removedEventNID)
		}
	}

	// Load the event JSON so we can look up the "membership" key.
	// TODO: Maybe add a membership key to the events table so we can load that
	// key without having to load the entire event JSON?
	events, err := updater.Events(ctx, "", eventNIDs)
	if err != nil {
		return nil, err
	}

	var updates []api.OutputEvent

	for _, change := range changes {
		var ae *types.Event
		var re *types.Event
		targetUserNID := change.EventStateKeyNID
		if change.removedEventNID != 0 {
			re, _ = helpers.EventMap(events).Lookup(change.removedEventNID)
		}
		if change.addedEventNID != 0 {
			ae, _ = helpers.EventMap(events).Lookup(change.addedEventNID)
		}
		if updates, err = r.updateMembership(ctx, updater, targetUserNID, re, ae, updates); err != nil {
			return nil, err
		}
	}
	return updates, nil
}

func (r *Inputer) updateMembership(
	ctx context.Context,
	updater *shared.RoomUpdater,
	targetUserNID types.EventStateKeyNID,
	remove, add *types.Event,
	updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	var err error
	// Default the membership to Leave if no event was added or removed.
	newMembership := spec.Leave
	if add != nil {
		newMembership, err = add.Membership()
		if err != nil {
			return nil, err
		}
	}

	var targetLocal bool
	if add != nil {
		targetLocal = r.isLocalTarget(ctx, add)
	}

	mu, err := updater.MembershipUpdater(targetUserNID, targetLocal)
	if err != nil {
		return nil, err
	}

	// In an ideal world, we shouldn't ever have "add" be nil and "remove" be
	// set, as this implies that we're deleting a state event without replacing
	// it (a thing that ordinarily shouldn't happen in Matrix). However, state
	// resets are sadly a thing occasionally and we have to account for that.
	// Beforehand there used to be a check here which stopped dead if we hit
	// this scenario, but that meant that the membership table got out of sync
	// after a state reset, often thinking that the user was still joined to
	// the room even though the room state said otherwise, and this would prevent
	// the user from being able to attempt to rejoin the room without modifying
	// the database. So instead we're going to remove the membership from the
	// database altogether, so that it doesn't create future problems.
	if add == nil && remove != nil {
		return nil, mu.Delete()
	}

	switch newMembership {
	case spec.Invite:
		return helpers.UpdateToInviteMembership(mu, add, updates, updater.RoomVersion())
	case spec.Join:
		return updateToJoinMembership(mu, add, updates)
	case spec.Leave, spec.Ban:
		return updateToLeaveMembership(mu, add, newMembership, updates)
	case spec.Knock:
		return updateToKnockMembership(mu, add, updates)
	default:
		panic(fmt.Errorf(
			"input: membership %q is not one of the allowed values", newMembership,
		))
	}
}

func (r *Inputer) isLocalTarget(ctx context.Context, event *types.Event) bool {
	isTargetLocalUser := false
	if statekey := event.StateKey(); statekey != nil {
		userID, err := r.Queryer.QueryUserIDForSender(ctx, event.RoomID(), spec.SenderID(*statekey))
		if err != nil || userID == nil {
			return isTargetLocalUser
		}
		isTargetLocalUser = userID.Domain() == r.ServerName
	}
	return isTargetLocalUser
}

func updateToJoinMembership(
	mu *shared.MembershipUpdater, add *types.Event, updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	// When we mark a user as being joined we will invalidate any invites that
	// are active for that user. We notify the consumers that the invites have
	// been retired using a special event, even though they could infer this
	// by studying the state changes in the room event stream.
	_, retired, err := mu.Update(tables.MembershipStateJoin, add)
	if err != nil {
		return nil, err
	}
	for _, eventID := range retired {
		updates = append(updates, api.OutputEvent{
			Type: api.OutputTypeRetireInviteEvent,
			RetireInviteEvent: &api.OutputRetireInviteEvent{
				EventID:          eventID,
				RoomID:           add.RoomID().String(),
				Membership:       spec.Join,
				RetiredByEventID: add.EventID(),
				TargetSenderID:   spec.SenderID(*add.StateKey()),
			},
		})
	}
	return updates, nil
}

func updateToLeaveMembership(
	mu *shared.MembershipUpdater, add *types.Event,
	newMembership string, updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	// When we mark a user as having left we will invalidate any invites that
	// are active for that user. We notify the consumers that the invites have
	// been retired using a special event, even though they could infer this
	// by studying the state changes in the room event stream.
	_, retired, err := mu.Update(tables.MembershipStateLeaveOrBan, add)
	if err != nil {
		return nil, err
	}
	for _, eventID := range retired {
		updates = append(updates, api.OutputEvent{
			Type: api.OutputTypeRetireInviteEvent,
			RetireInviteEvent: &api.OutputRetireInviteEvent{
				EventID:          eventID,
				RoomID:           add.RoomID().String(),
				Membership:       newMembership,
				RetiredByEventID: add.EventID(),
				TargetSenderID:   spec.SenderID(*add.StateKey()),
			},
		})
	}
	return updates, nil
}

func updateToKnockMembership(
	mu *shared.MembershipUpdater, add *types.Event, updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	if _, _, err := mu.Update(tables.MembershipStateKnock, add); err != nil {
		return nil, err
	}
	return updates, nil
}

// membershipChanges pairs up the membership state changes.
func membershipChanges(removed, added []types.StateEntry) []stateChange {
	changes := pairUpChanges(removed, added)
	var result []stateChange
	for _, c := range changes {
		if c.EventTypeNID == types.MRoomMemberNID {
			result = append(result, c)
		}
	}
	return result
}

type stateChange struct {
	types.StateKeyTuple
	removedEventNID types.EventNID
	addedEventNID   types.EventNID
}

// pairUpChanges pairs up the state events added and removed for each type,
// state key tuple.
func pairUpChanges(removed, added []types.StateEntry) []stateChange {
	tuples := make(map[types.StateKeyTuple]stateChange)
	changes := []stateChange{}

	// First, go through the newly added state entries.
	for _, add := range added {
		if change, ok := tuples[add.StateKeyTuple]; ok {
			// If we already have an entry, update it.
			change.addedEventNID = add.EventNID
			tuples[add.StateKeyTuple] = change
		} else {
			// Otherwise, create a new entry.
			tuples[add.StateKeyTuple] = stateChange{add.StateKeyTuple, 0, add.EventNID}
		}
	}

	// Now go through the removed state entries.
	for _, remove := range removed {
		if change, ok := tuples[remove.StateKeyTuple]; ok {
			// If we already have an entry, update it.
			change.removedEventNID = remove.EventNID
			tuples[remove.StateKeyTuple] = change
		} else {
			// Otherwise, create a new entry.
			tuples[remove.StateKeyTuple] = stateChange{remove.StateKeyTuple, remove.EventNID, 0}
		}
	}

	// Now return the changes as an array.
	for _, change := range tuples {
		changes = append(changes, change)
	}

	return changes
}
