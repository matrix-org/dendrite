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
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
)

func init() {
	prometheus.MustRegister(calculateHistoryVisibilityDuration)
}

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
// Rules as defined by https://spec.matrix.org/v1.3/client-server-api/#server-behaviour-5
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
	syncDB storage.DatabaseTransaction,
	rsAPI api.SyncRoomserverAPI,
	events []*gomatrixserverlib.HeaderedEvent,
	alwaysIncludeEventIDs map[string]struct{},
	userID, endpoint string,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	if len(events) == 0 {
		return events, nil
	}
	start := time.Now()

	// try to get the current membership of the user
	membershipCurrent, _, err := syncDB.SelectMembershipForUser(ctx, events[0].RoomID(), userID, math.MaxInt64)
	if err != nil {
		return nil, err
	}

	// Get the mapping from eventID -> eventVisibility
	eventsFiltered := make([]*gomatrixserverlib.HeaderedEvent, 0, len(events))
	visibilities, err := visibilityForEvents(ctx, rsAPI, events, userID, events[0].RoomID())
	if err != nil {
		return eventsFiltered, err
	}
	for _, ev := range events {
		evVis := visibilities[ev.EventID()]
		evVis.membershipCurrent = membershipCurrent
		// Always include specific state events for /sync responses
		if alwaysIncludeEventIDs != nil {
			if _, ok := alwaysIncludeEventIDs[ev.EventID()]; ok {
				eventsFiltered = append(eventsFiltered, ev)
				continue
			}
		}
		// NOTSPEC: Always allow user to see their own membership events (spec contains more "rules")
		if ev.Type() == gomatrixserverlib.MRoomMember && ev.StateKeyEquals(userID) {
			eventsFiltered = append(eventsFiltered, ev)
			continue
		}
		// Always allow history evVis events on boundaries. This is done
		// by setting the effective evVis to the least restrictive
		// of the old vs new.
		// https://spec.matrix.org/v1.3/client-server-api/#server-behaviour-5
		if hisVis, err := ev.HistoryVisibility(); err == nil {
			prevHisVis := gjson.GetBytes(ev.Unsigned(), "prev_content.history_visibility").String()
			oldPrio, ok := historyVisibilityPriority[gomatrixserverlib.HistoryVisibility(prevHisVis)]
			// if we can't get the previous history visibility, default to shared.
			if !ok {
				oldPrio = historyVisibilityPriority[gomatrixserverlib.HistoryVisibilityShared]
			}
			// no OK check, since this should have been validated when setting the value
			newPrio := historyVisibilityPriority[hisVis]
			if oldPrio < newPrio {
				evVis.visibility = gomatrixserverlib.HistoryVisibility(prevHisVis)
			}
		}
		// do the actual check
		allowed := evVis.allowed()
		if allowed {
			eventsFiltered = append(eventsFiltered, ev)
		}
	}
	calculateHistoryVisibilityDuration.With(prometheus.Labels{"api": endpoint}).Observe(float64(time.Since(start).Milliseconds()))
	return eventsFiltered, nil
}

// visibilityForEvents returns a map from eventID to eventVisibility containing the visibility and the membership
// of `userID` at the given event.
// Returns an error if the roomserver can't calculate the memberships.
func visibilityForEvents(
	ctx context.Context,
	rsAPI api.SyncRoomserverAPI,
	events []*gomatrixserverlib.HeaderedEvent,
	userID, roomID string,
) (map[string]eventVisibility, error) {
	eventIDs := make([]string, len(events))
	for i := range events {
		eventIDs[i] = events[i].EventID()
	}

	result := make(map[string]eventVisibility, len(eventIDs))

	// get the membership events for all eventIDs
	membershipResp := &api.QueryMembershipAtEventResponse{}
	err := rsAPI.QueryMembershipAtEvent(ctx, &api.QueryMembershipAtEventRequest{
		RoomID:   roomID,
		EventIDs: eventIDs,
		UserID:   userID,
	}, membershipResp)
	if err != nil {
		logrus.WithError(err).Error("visibilityForEvents: failed to fetch membership at event, defaulting to 'leave'")
	}

	// Create a map from eventID -> eventVisibility
	for _, event := range events {
		eventID := event.EventID()
		vis := eventVisibility{
			membershipAtEvent: gomatrixserverlib.Leave, // default to leave, to not expose events by accident
			visibility:        event.Visibility,
		}
		membershipEvs, ok := membershipResp.Memberships[eventID]
		if !ok {
			result[eventID] = vis
			continue
		}
		for _, ev := range membershipEvs {
			membership, err := ev.Membership()
			if err != nil {
				return result, err
			}
			vis.membershipAtEvent = membership
		}
		result[eventID] = vis
	}
	return result, nil
}
