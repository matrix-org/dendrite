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
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

func updateMemberships(
	db RoomEventDatabase, updater types.RoomRecentEventsUpdater, removed, added []types.StateEntry,
) ([]api.OutputEvent, error) {
	changes := membershipChanges(removed, added)
	var eventNIDs []types.EventNID
	for _, change := range changes {
		if change.added.EventNID != 0 {
			eventNIDs = append(eventNIDs, change.added.EventNID)
		}
		if change.removed.EventNID != 0 {
			eventNIDs = append(eventNIDs, change.removed.EventNID)
		}
	}
	events, err := db.Events(eventNIDs)
	if err != nil {
		return nil, err
	}

	var updates []api.OutputEvent

	for _, change := range changes {
		var ae *gomatrixserverlib.Event
		var re *gomatrixserverlib.Event
		var targetNID types.EventStateKeyNID
		if change.removed.EventNID != 0 {
			ev, _ := eventMap(events).lookup(change.removed.EventNID)
			if ev != nil {
				re = &ev.Event
			}
			targetNID = change.removed.EventStateKeyNID
		}
		if change.added.EventNID != 0 {
			ev, _ := eventMap(events).lookup(change.added.EventNID)
			if ev != nil {
				ae = &ev.Event
			}
			targetNID = change.added.EventStateKeyNID
		}
		if updates, err = updateMembership(updater, targetNID, re, ae, updates); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func updateMembership(
	updater types.RoomRecentEventsUpdater, targetNID types.EventStateKeyNID,
	remove *gomatrixserverlib.Event, add *gomatrixserverlib.Event,
	updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	var err error
	old := "leave"
	new := "leave"

	if remove != nil {
		old, err = remove.Membership()
		if err != nil {
			return nil, err
		}
	}
	if add != nil {
		new, err = add.Membership()
		if err != nil {
			return nil, err
		}
	}
	if old == new {
		return updates, nil
	}

	mu, err := updater.MembershipUpdater(targetNID)
	if err != nil {
		return nil, err
	}

	return updateWithNewMembership(mu, new, add, updates)
}

func updateWithNewMembership(
	mu types.MembershipUpdater, newMembership string, add *gomatrixserverlib.Event,
	updates []api.OutputEvent,
) ([]api.OutputEvent, error) {
	switch newMembership {
	case "invite":
		needsSending, err := mu.SetToInvite(*add)
		if err != nil {
			return nil, err
		}
		if needsSending {
			updates = appendInviteUpdate(updates, add)
		}
		return updates, nil
	case "join":
		if mu.IsJoin() {
			return updates, nil
		}
		retired, err := mu.SetToJoin(add.Sender())
		if err != nil {
			return nil, err
		}
		return appendRetireUpdates(updates, add, newMembership, retired), nil
	case "leave", "ban":
		if mu.IsLeave() {
			return updates, nil
		}
		retired, err := mu.SetToLeave(add.Sender())
		if err != nil {
			mu.SetToLeave(add.Sender())
		}
		return appendRetireUpdates(updates, add, newMembership, retired), nil
	default:
		panic(fmt.Errorf(
			"input: membership %q is not one of the allowed values", newMembership,
		))
	}
}

func appendInviteUpdate(
	updates []api.OutputEvent, add *gomatrixserverlib.Event,
) []api.OutputEvent {
	onie := api.OutputNewInviteEvent{
		Event: *add,
	}
	return append(updates, api.OutputEvent{
		Type:           api.OutputTypeNewInviteEvent,
		NewInviteEvent: &onie,
	})
}

func appendRetireUpdates(
	updates []api.OutputEvent, add *gomatrixserverlib.Event, membership string, retired []string,
) []api.OutputEvent {
	for _, eventID := range retired {
		orie := api.OutputRetireInviteEvent{
			EventID:    eventID,
			Membership: membership,
		}
		if add != nil {
			orie.RetiredByEventID = add.EventID()
		}
		updates = append(updates, api.OutputEvent{
			Type:              api.OutputTypeRetireInviteEvent,
			RetireInviteEvent: &orie,
		})
	}
	return updates
}

type stateChange struct {
	removed types.StateEntry
	added   types.StateEntry
}

func pairUpChanges(removed, added []types.StateEntry) []stateChange {
	var ai int
	var ri int
	var result []stateChange
	for {
		switch {
		case ai == len(added):
			for _, s := range removed[ri:] {
				result = append(result, stateChange{removed: s})
			}
			return result
		case ri == len(removed):
			for _, s := range added[ai:] {
				result = append(result, stateChange{added: s})
			}
			return result
		case added[ai].StateKeyTuple == removed[ri].StateKeyTuple:
			result = append(result, stateChange{
				removed: removed[ri],
				added:   added[ai],
			})
			ai++
			ri++
		case added[ai].StateKeyTuple.LessThan(removed[ri].StateKeyTuple):
			result = append(result, stateChange{added: added[ai]})
			ai++
		default:
			result = append(result, stateChange{removed: removed[ri]})
			ri++
		}
	}
}

// membershipChanges pairs up the membership state changes from a sorted list
// of state removed and a sorted list of state added.
func membershipChanges(removed, added []types.StateEntry) []stateChange {
	changes := pairUpChanges(removed, added)
	var result []stateChange
	for _, c := range changes {
		if c.added.EventTypeNID == types.MRoomMemberNID ||
			c.removed.EventTypeNID == types.MRoomMemberNID {
			result = append(result, c)
		}
	}
	return result
}
