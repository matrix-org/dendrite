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

package input

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// updateMembership updates the current membership and the invites for each
// user affected by a change in the current state of the room.
// Returns a list of output events to write to the kafka log to inform the
// consumers about the invites added or retired by the change in current state.
func updateMemberships(
	ctx context.Context,
	db storage.Database,
	updater types.RoomRecentEventsUpdater,
	removed, added []types.StateEntry,
) ([]api.OutputEvent, error) {
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
	events, err := db.Events(ctx, eventNIDs)
	if err != nil {
		return nil, err
	}

	var updates []api.OutputEvent

	for _, change := range changes {
		var ae *gomatrixserverlib.Event
		var re *gomatrixserverlib.Event
		targetUserNID := change.EventStateKeyNID
		if change.removedEventNID != 0 {
			ev, _ := eventMap(events).lookup(change.removedEventNID)
			if ev != nil {
				re = &ev.Event
			}
		}
		if change.addedEventNID != 0 {
			ev, _ := eventMap(events).lookup(change.addedEventNID)
			if ev != nil {
				ae = &ev.Event
			}
		}
		if updates, err = updateMembership(updater, targetUserNID, re, ae, updates); err != nil {
			return nil, err
		}
	}
	return updates, nil
}

func updateMembership(
	updater types.RoomRecentEventsUpdater, targetUserNID types.EventStateKeyNID,
	remove, add *gomatrixserverlib.Event,
	updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	var err error
	// Default the membership to Leave if no event was added or removed.
	oldMembership := gomatrixserverlib.Leave
	newMembership := gomatrixserverlib.Leave

	if remove != nil {
		oldMembership, err = remove.Membership()
		if err != nil {
			return nil, err
		}
	}
	if add != nil {
		newMembership, err = add.Membership()
		if err != nil {
			return nil, err
		}
	}
	if oldMembership == newMembership && newMembership != gomatrixserverlib.Join {
		// If the membership is the same then nothing changed and we can return
		// immediately, unless it's a Join update (e.g. profile update).
		return updates, nil
	}

	mu, err := updater.MembershipUpdater(targetUserNID)
	if err != nil {
		return nil, err
	}

	switch newMembership {
	case gomatrixserverlib.Invite:
		return updateToInviteMembership(mu, add, updates, updater.RoomVersion())
	case gomatrixserverlib.Join:
		return updateToJoinMembership(mu, add, updates)
	case gomatrixserverlib.Leave, gomatrixserverlib.Ban:
		return updateToLeaveMembership(mu, add, newMembership, updates)
	default:
		panic(fmt.Errorf(
			"input: membership %q is not one of the allowed values", newMembership,
		))
	}
}

func updateToInviteMembership(
	mu types.MembershipUpdater, add *gomatrixserverlib.Event, updates []api.OutputEvent,
	roomVersion gomatrixserverlib.RoomVersion,
) ([]api.OutputEvent, error) {
	// We may have already sent the invite to the user, either because we are
	// reprocessing this event, or because the we received this invite from a
	// remote server via the federation invite API. In those cases we don't need
	// to send the event.
	needsSending, err := mu.SetToInvite(*add)
	if err != nil {
		return nil, err
	}
	if needsSending {
		// We notify the consumers using a special event even though we will
		// notify them about the change in current state as part of the normal
		// room event stream. This ensures that the consumers only have to
		// consider a single stream of events when determining whether a user
		// is invited, rather than having to combine multiple streams themselves.
		onie := api.OutputNewInviteEvent{
			Event:       (*add).Headered(roomVersion),
			RoomVersion: roomVersion,
		}
		updates = append(updates, api.OutputEvent{
			Type:           api.OutputTypeNewInviteEvent,
			NewInviteEvent: &onie,
		})
	}
	return updates, nil
}

func updateToJoinMembership(
	mu types.MembershipUpdater, add *gomatrixserverlib.Event, updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	// If the user is already marked as being joined, we call SetToJoin to update
	// the event ID then we can return immediately. Retired is ignored as there
	// is no invite event to retire.
	if mu.IsJoin() {
		_, err := mu.SetToJoin(add.Sender(), add.EventID(), true)
		if err != nil {
			return nil, err
		}
		return updates, nil
	}
	// When we mark a user as being joined we will invalidate any invites that
	// are active for that user. We notify the consumers that the invites have
	// been retired using a special event, even though they could infer this
	// by studying the state changes in the room event stream.
	retired, err := mu.SetToJoin(add.Sender(), add.EventID(), false)
	if err != nil {
		return nil, err
	}
	for _, eventID := range retired {
		orie := api.OutputRetireInviteEvent{
			EventID:          eventID,
			Membership:       gomatrixserverlib.Join,
			RetiredByEventID: add.EventID(),
			TargetUserID:     *add.StateKey(),
		}
		updates = append(updates, api.OutputEvent{
			Type:              api.OutputTypeRetireInviteEvent,
			RetireInviteEvent: &orie,
		})
	}
	return updates, nil
}

func updateToLeaveMembership(
	mu types.MembershipUpdater, add *gomatrixserverlib.Event,
	newMembership string, updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	// If the user is already neither joined, nor invited to the room then we
	// can return immediately.
	if mu.IsLeave() {
		return updates, nil
	}
	// When we mark a user as having left we will invalidate any invites that
	// are active for that user. We notify the consumers that the invites have
	// been retired using a special event, even though they could infer this
	// by studying the state changes in the room event stream.
	retired, err := mu.SetToLeave(add.Sender(), add.EventID())
	if err != nil {
		return nil, err
	}
	for _, eventID := range retired {
		orie := api.OutputRetireInviteEvent{
			EventID:          eventID,
			Membership:       newMembership,
			RetiredByEventID: add.EventID(),
			TargetUserID:     *add.StateKey(),
		}
		updates = append(updates, api.OutputEvent{
			Type:              api.OutputTypeRetireInviteEvent,
			RetireInviteEvent: &orie,
		})
	}
	return updates, nil
}

// membershipChanges pairs up the membership state changes from a sorted list
// of state removed and a sorted list of state added.
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
// state key tuple. Assumes that removed and added are sorted.
func pairUpChanges(removed, added []types.StateEntry) []stateChange {
	var ai int
	var ri int
	var result []stateChange
	for {
		switch {
		case ai == len(added):
			// We've reached the end of the added entries.
			// The rest of the removed list are events that were removed without
			// an event with the same state key being added.
			for _, s := range removed[ri:] {
				result = append(result, stateChange{
					StateKeyTuple:   s.StateKeyTuple,
					removedEventNID: s.EventNID,
				})
			}
			return result
		case ri == len(removed):
			// We've reached the end of the removed entries.
			// The rest of the added list are events that were added without
			// an event with the same state key being removed.
			for _, s := range added[ai:] {
				result = append(result, stateChange{
					StateKeyTuple: s.StateKeyTuple,
					addedEventNID: s.EventNID,
				})
			}
			return result
		case added[ai].StateKeyTuple == removed[ri].StateKeyTuple:
			// The tuple is in both lists so an event with that key is being
			// removed and another event with the same key is being added.
			result = append(result, stateChange{
				StateKeyTuple:   added[ai].StateKeyTuple,
				removedEventNID: removed[ri].EventNID,
				addedEventNID:   added[ai].EventNID,
			})
			ai++
			ri++
		case added[ai].StateKeyTuple.LessThan(removed[ri].StateKeyTuple):
			// The lists are sorted so the added entry being less than the
			// removed entry means that the added event was added without an
			// event with the same key being removed.
			result = append(result, stateChange{
				StateKeyTuple: added[ai].StateKeyTuple,
				addedEventNID: added[ai].EventNID,
			})
			ai++
		default:
			// Reaching the default case implies that the removed entry is less
			// than the added entry. Since the lists are sorted this means that
			// the removed event was removed without an event with the same
			// key being added.
			result = append(result, stateChange{
				StateKeyTuple:   removed[ai].StateKeyTuple,
				removedEventNID: removed[ri].EventNID,
			})
			ri++
		}
	}
}
