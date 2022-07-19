// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package internal

import (
	"context"
	"math"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
)

var historyVisibilityPriority = map[gomatrixserverlib.HistoryVisibility]uint8{
	gomatrixserverlib.WorldReadable:            0,
	gomatrixserverlib.HistoryVisibilityShared:  1,
	gomatrixserverlib.HistoryVisibilityInvited: 2,
	gomatrixserverlib.HistoryVisibilityJoined:  3,
}

// eventVisibility contains the history visibility and membership state at a given event
type eventVisibility struct {
	visibility        gomatrixserverlib.HistoryVisibility
	membershipAtEvent string
	membershipCurrent string
}

// allowed checks the eventVisibility if the user is allowed to see the event.
func (ev eventVisibility) allowed() (allowed bool) {
	switch ev.visibility {
	case gomatrixserverlib.HistoryVisibilityWorldReadable:
		// If the history_visibility was set to world_readable, allow.
		return true
	case gomatrixserverlib.HistoryVisibilityJoined:
		// If the user’s membership was join, allow.
		if ev.membershipAtEvent == gomatrixserverlib.Join {
			return true
		}
		return false
	case gomatrixserverlib.HistoryVisibilityShared:
		// If the user’s membership was join, allow.
		// If history_visibility was set to shared, and the user joined the room at any point after the event was sent, allow.
		if ev.membershipAtEvent == gomatrixserverlib.Join || ev.membershipCurrent == gomatrixserverlib.Join {
			return true
		}
		return false
	case gomatrixserverlib.HistoryVisibilityInvited:
		// If the user’s membership was join, allow.
		if ev.membershipAtEvent == gomatrixserverlib.Join {
			return true
		}
		if ev.membershipAtEvent == gomatrixserverlib.Invite {
			return true
		}
		return false
	default:
		return false
	}
}

// ApplyHistoryVisibilityFilter applies the room history visibility filter on gomatrixserverlib.HeaderedEvents.
// Returns the filtered events and an error, if any.
func ApplyHistoryVisibilityFilter(
	ctx context.Context,
	syncDB storage.Database,
	events []*gomatrixserverlib.HeaderedEvent,
	alwaysIncludeEventIDs map[string]struct{},
	userID string,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	eventsFiltered := make([]*gomatrixserverlib.HeaderedEvent, 0, len(events))
	if len(events) == 0 {
		return events, nil
	}

	// try to get the current membership of the user
	membershipCurrent, _, err := syncDB.SelectMembershipForUser(ctx, events[0].RoomID(), userID, math.MaxInt64)
	if err != nil {
		return nil, err
	}

	for _, ev := range events {
		event, err := visibilityForEvent(ctx, syncDB, ev, userID)
		if err != nil {
			return eventsFiltered, err
		}
		event.membershipCurrent = membershipCurrent
		// Always include specific state events for /sync responses
		if alwaysIncludeEventIDs != nil {
			if _, ok := alwaysIncludeEventIDs[ev.EventID()]; ok {
				eventsFiltered = append(eventsFiltered, ev)
				continue
			}
		}
		// NOTSPEC: Always allow user to see their own membership events (spec contains more "rules")
		if ev.Type() == gomatrixserverlib.MRoomMember && ev.StateKey() != nil && *ev.StateKey() == userID {
			eventsFiltered = append(eventsFiltered, ev)
			continue
		}
		// Handle history visibility changes
		if hisVis, err := ev.HistoryVisibility(); err == nil {
			prevHisVis := gjson.GetBytes(ev.Unsigned(), "prev_content.history_visibility").String()
			if oldPrio, ok := historyVisibilityPriority[gomatrixserverlib.HistoryVisibility(prevHisVis)]; ok {
				// no OK check, since this should have been validated when setting the value
				newPrio := historyVisibilityPriority[hisVis]
				if oldPrio < newPrio {
					event.visibility = gomatrixserverlib.HistoryVisibility(prevHisVis)
				}
			}
		}
		// do the actual check
		allowed := event.allowed()
		if allowed {
			eventsFiltered = append(eventsFiltered, ev)
		}
	}
	return eventsFiltered, nil
}

// visibilityForEvent returns an eventVisibility containing the visibility and the membership at the given event.
// Returns an error if the database returns an error.
func visibilityForEvent(ctx context.Context, db storage.Database, event *gomatrixserverlib.HeaderedEvent, userID string) (eventVisibility, error) {
	// get the membership event
	var membershipAtEvent string
	membershipAtEvent, _, err := db.SelectMembershipForUser(ctx, event.RoomID(), userID, event.Depth())
	if err != nil {
		return eventVisibility{}, err
	}

	return eventVisibility{
		visibility:        event.Visibility,
		membershipAtEvent: membershipAtEvent,
	}, nil
}
