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
	"database/sql"
	"fmt"
	"math"

	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

type HistoryVisibility string

const (
	WorldReadable HistoryVisibility = "world_readable"
	Joined        HistoryVisibility = "joined"
	Shared        HistoryVisibility = "shared"
	Default       HistoryVisibility = "default"
	Invited       HistoryVisibility = "invited"
)

var historyVisibilityPriority = map[HistoryVisibility]uint8{
	WorldReadable: 0,
	Shared:        1,
	Default:       1, // as per the spec, default == shared
	Invited:       2,
	Joined:        3,
}

// EventVisibility contains the history visibility and membership state at a given event
type EventVisibility struct {
	Visibility        HistoryVisibility
	MembershipAtEvent string
	MembershipCurrent string
}

// Visibility is a map from event_id to EvVis, which contains the history visibility and membership for a given user.
type Visibility map[string]EventVisibility

// allowed checks the Visibility map if the user is allowed to see the given event.
func (v Visibility) allowed(eventID string) (allowed bool) {
	ev, ok := v[eventID]
	if !ok {
		return false
	}
	switch ev.Visibility {
	case WorldReadable:
		// If the history_visibility was set to world_readable, allow.
		return true
	case Joined:
		// If the user’s membership was join, allow.
		if ev.MembershipAtEvent == gomatrixserverlib.Join {
			return true
		}
		return false
	case Shared, Default:
		// If the user’s membership was join, allow.
		// If history_visibility was set to shared, and the user joined the room at any point after the event was sent, allow.
		if ev.MembershipAtEvent == gomatrixserverlib.Join || ev.MembershipCurrent == gomatrixserverlib.Join {
			return true
		}
		return false
	case Invited:
		// If the user’s membership was join, allow.
		if ev.MembershipAtEvent == gomatrixserverlib.Join {
			return true
		}
		if ev.MembershipAtEvent == gomatrixserverlib.Invite {
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
	stateForEvents, err := getStateForEvents(ctx, syncDB, events, userID)
	if err != nil {
		return eventsFiltered, err
	}
	for _, ev := range events {
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
			if oldPrio, ok := historyVisibilityPriority[HistoryVisibility(prevHisVis)]; ok {
				// no OK check, since this should have been validated when setting the value
				newPrio := historyVisibilityPriority[HistoryVisibility(hisVis)]
				if oldPrio < newPrio {
					sfe := stateForEvents[ev.EventID()]
					sfe.Visibility = HistoryVisibility(prevHisVis)
					stateForEvents[ev.EventID()] = sfe
				}
			}
		}
		// do the actual check
		if stateForEvents.allowed(ev.EventID()) {
			eventsFiltered = append(eventsFiltered, ev)
		}
	}
	return eventsFiltered, nil
}

// getStateForEvents returns a Visibility map containing the state before and at the given events.
func getStateForEvents(ctx context.Context, db storage.Database, events []*gomatrixserverlib.HeaderedEvent, userID string) (Visibility, error) {
	result := make(map[string]EventVisibility, len(events))
	if len(events) == 0 {
		return result, nil
	}
	var (
		membershipCurrent string
		err               error
	)
	// try to get the current membership of the user
	membershipCurrent, _, err = db.SelectMembershipForUser(ctx, events[0].RoomID(), userID, math.MaxInt64)
	if err != nil {
		return nil, err
	}

	for _, ev := range events {
		// get the event topology position
		pos, err := db.EventPositionInTopology(ctx, ev.EventID())
		if err != nil {
			return nil, fmt.Errorf("initial event does not exist: %w", err)
		}
		// By default if no history_visibility is set, or if the value is not understood, the visibility is assumed to be shared
		var hisVis = "shared"
		historyEvent, _, err := db.SelectTopologicalEvent(ctx, int(pos.Depth), "m.room.history_visibility", ev.RoomID())
		if err != nil {
			if err != sql.ErrNoRows {
				return nil, err
			}
			logrus.WithError(err).Debugf("unable to get history event, defaulting to %s", Shared)
		} else {
			hisVis, err = historyEvent.HistoryVisibility()
			if err != nil {
				hisVis = "shared"
			}
		}
		// get the membership event
		var membership string
		membership, _, err = db.SelectMembershipForUser(ctx, ev.RoomID(), userID, int64(pos.Depth))
		if err != nil {
			return nil, err
		}
		// finally create the mapping
		result[ev.EventID()] = EventVisibility{
			Visibility:        HistoryVisibility(hisVis),
			MembershipAtEvent: membership,
			MembershipCurrent: membershipCurrent,
		}
	}

	return result, nil
}
