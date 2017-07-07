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
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

func updateMemberships(
	db RoomEventDatabase, updater types.RoomRecentEventsUpdater, removed, added []types.StateEntry,
) error {
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
		return err
	}

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
			if ae != nil {
				ae = &ev.Event
			}
			targetNID = change.added.EventStateKeyNID
		}
		if err := updateMembership(updater, targetNID, re, ae); err != nil {
			return err
		}
	}
	return nil
}

func updateMembership(
	updater types.RoomRecentEventsUpdater, targetNID types.EventStateKeyNID, remove *gomatrixserverlib.Event, add *gomatrixserverlib.Event,
) error {
	old := "leave"
	new := "leave"

	if remove != nil {
		membership, err := remove.Membership()
		if err != nil {
			return err
		}
		old = *membership
	}
	if add != nil {
		membership, err := add.Membership()
		if err != nil {
			return err
		}
		new = *membership
	}
	if old == new {
		return nil
	}

	mu, err := updater.MembershipUpdater(targetNID)
	if err != nil {
		return err
	}

	switch new {
	case "invite":
		_, err := mu.SetToInvite(*add)
		if err != nil {
			return err
		}
	case "join":
		if !mu.IsJoin() {
			mu.SetToJoin(add.Sender())
		}
	case "leave":
		if !mu.IsLeave() {
			mu.SetToLeave(add.Sender())
		}
	}
	return nil
}

type stateChange struct {
	removed types.StateEntry
	added   types.StateEntry
}

func membershipChanges(removed, added []types.StateEntry) []stateChange {
	var ai int
	var ri int
	var result []stateChange
	for {
		switch {
		case ai == len(added):
			for _, s := range removed[ri:] {
				if s.EventTypeNID == types.MRoomMemberNID {
					result = append(result, stateChange{removed: s})
				}
			}
			return result
		case ri == len(removed):
			for _, s := range removed[ai:] {
				if s.EventTypeNID == types.MRoomMemberNID {
					result = append(result, stateChange{added: s})
				}
			}
			return result
		case added[ai].EventTypeNID != types.MRoomMemberNID:
			ai++
		case removed[ri].EventTypeNID != types.MRoomMemberNID:
			ri++
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
