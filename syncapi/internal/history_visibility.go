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
	"sync"
	"time"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tidwall/gjson"
)

var registerOnce = &sync.Once{}

// calculateHistoryVisibilityDuration stores the time it takes to
// calculate the history visibility. In polylith mode the roundtrip
// to the roomserver is included in this time.
var calculateHistoryVisibilityDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace: "dendrite",
		Subsystem: "syncapi",
		Name:      "calculateHistoryVisibility_duration_millis",
		Help:      "How long it takes to calculate the history visibility",
		Buckets: []float64{ // milliseconds
			5, 10, 25, 50, 75, 100, 250, 500,
			1000, 2000, 3000, 4000, 5000, 6000,
			7000, 8000, 9000, 10000, 15000, 20000,
		},
	},
	[]string{"api"},
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
	rsAPI api.SyncRoomserverAPI,
	events []*gomatrixserverlib.HeaderedEvent,
	alwaysIncludeEventIDs map[string]struct{},
	userID, endpoint string,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	registerOnce.Do(func() {
		prometheus.MustRegister(calculateHistoryVisibilityDuration)
	})
	if len(events) == 0 {
		return events, nil
	}
	start := time.Now()

	// try to get the current membership of the user
	membershipCurrent, _, err := syncDB.SelectMembershipForUser(ctx, events[0].RoomID(), userID, math.MaxInt64)
	if err != nil {
		return nil, err
	}

	eventIDs := make([]string, len(events))
	for i := range events {
		eventIDs[i] = events[i].EventID()
	}

	// Get the mapping from eventID -> eventVisibility
	eventsFiltered := make([]*gomatrixserverlib.HeaderedEvent, 0, len(events))
	event, err := visibilityForEvents(ctx, rsAPI, eventIDs, userID, events[0].RoomID())
	if err != nil {
		return eventsFiltered, err
	}
	for _, ev := range events {
		d := event[ev.EventID()]
		d.membershipCurrent = membershipCurrent
		d.visibility = ev.Visibility
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
					d.visibility = gomatrixserverlib.HistoryVisibility(prevHisVis)
				}
			}
		}
		// do the actual check
		allowed := d.allowed()
		if allowed {
			eventsFiltered = append(eventsFiltered, ev)
		}
	}
	calculateHistoryVisibilityDuration.With(prometheus.Labels{"api": endpoint}).Observe(float64(time.Since(start).Milliseconds()))
	return eventsFiltered, nil
}

// visibilityForEvents returns a map from eventID to eventVisibility containing the visibility and the membership at the given event.
// Returns an error if the roomserver can't calculate the memberships.
func visibilityForEvents(ctx context.Context, rsAPI api.SyncRoomserverAPI, eventIDs []string, userID, roomID string) (map[string]eventVisibility, error) {
	res := make(map[string]eventVisibility, len(eventIDs))

	// get the membership events for all eventIDs
	resp := &api.QueryMembersipAtEventResponse{}
	err := rsAPI.QueryMembershipAtEvent(ctx, &api.QueryMembersipAtEventRequest{
		RoomID:   roomID,
		EventIDs: eventIDs,
		UserID:   userID,
	}, resp)
	if err != nil {
		return res, err
	}

	// Create a map from eventID -> eventVisibility
	for _, eventID := range eventIDs {
		vis := eventVisibility{membershipAtEvent: gomatrixserverlib.Leave}
		events, ok := resp.Memberships[eventID]
		if !ok {
			res[eventID] = vis
			continue
		}
		for _, ev := range events {
			membership, err := ev.Membership()
			if err != nil {
				return res, err
			}
			vis.membershipAtEvent = membership
		}
		res[eventID] = vis
	}

	return res, nil
}
