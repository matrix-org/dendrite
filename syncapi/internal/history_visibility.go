// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package internal

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/syncapi/storage"
)

func init() {
	prometheus.MustRegister(calculateHistoryVisibilityDuration)
}

// calculateHistoryVisibilityDuration stores the time it takes to
// calculate the history visibility.
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
	spec.WorldReadable:                         0,
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
		if ev.membershipAtEvent == spec.Join {
			return true
		}
		return false
	case gomatrixserverlib.HistoryVisibilityShared:
		// If the user’s membership was join, allow.
		// If history_visibility was set to shared, and the user joined the room at any point after the event was sent, allow.
		if ev.membershipAtEvent == spec.Join || ev.membershipCurrent == spec.Join {
			return true
		}
		return false
	case gomatrixserverlib.HistoryVisibilityInvited:
		// If the user’s membership was join, allow.
		if ev.membershipAtEvent == spec.Join {
			return true
		}
		if ev.membershipAtEvent == spec.Invite {
			return true
		}
		return false
	default:
		return false
	}
}

// ApplyHistoryVisibilityFilter applies the room history visibility filter on types.HeaderedEvents.
// Returns the filtered events and an error, if any.
//
// This function assumes that all provided events are from the same room.
func ApplyHistoryVisibilityFilter(
	ctx context.Context,
	syncDB storage.DatabaseTransaction,
	rsAPI api.SyncRoomserverAPI,
	events []*types.HeaderedEvent,
	alwaysIncludeEventIDs map[string]struct{},
	userID spec.UserID, endpoint string,
) ([]*types.HeaderedEvent, error) {
	if len(events) == 0 {
		return events, nil
	}
	start := time.Now()

	// try to get the current membership of the user
	membershipCurrent, _, err := syncDB.SelectMembershipForUser(ctx, events[0].RoomID().String(), userID.String(), math.MaxInt64)
	if err != nil {
		return nil, err
	}

	// Get the mapping from eventID -> eventVisibility
	eventsFiltered := make([]*types.HeaderedEvent, 0, len(events))
	firstEvRoomID := events[0].RoomID()
	senderID, err := rsAPI.QuerySenderIDForUser(ctx, firstEvRoomID, userID)
	if err != nil {
		return nil, err
	}
	visibilities := visibilityForEvents(ctx, rsAPI, events, senderID, firstEvRoomID)

	for _, ev := range events {
		// Validate same room assumption
		if ev.RoomID().String() != firstEvRoomID.String() {
			return nil, fmt.Errorf("events from different rooms supplied to ApplyHistoryVisibilityFilter")
		}

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
		if senderID != nil {
			if ev.Type() == spec.MRoomMember && ev.StateKeyEquals(string(*senderID)) {
				eventsFiltered = append(eventsFiltered, ev)
				continue
			}
		}

		// Always allow history evVis events on boundaries. This is done
		// by setting the effective evVis to the least restrictive
		// of the old vs new.
		// https://spec.matrix.org/v1.3/client-server-api/#server-behaviour-5
		if ev.Type() == spec.MRoomHistoryVisibility {
			hisVis, err := ev.HistoryVisibility()

			if err == nil && hisVis != "" {
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
				} else {
					evVis.visibility = hisVis
				}
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
// of `senderID` at the given event. If provided sender ID is nil, assume that membership is Leave
// Returns an error if the roomserver can't calculate the memberships.
func visibilityForEvents(
	ctx context.Context,
	rsAPI api.SyncRoomserverAPI,
	events []*types.HeaderedEvent,
	senderID *spec.SenderID, roomID spec.RoomID,
) map[string]eventVisibility {
	eventIDs := make([]string, len(events))
	for i := range events {
		eventIDs[i] = events[i].EventID()
	}

	result := make(map[string]eventVisibility, len(eventIDs))

	// get the membership events for all eventIDs
	var err error
	membershipEvents := make(map[string]*types.HeaderedEvent)
	if senderID != nil {
		membershipEvents, err = rsAPI.QueryMembershipAtEvent(ctx, roomID, eventIDs, *senderID)
		if err != nil {
			logrus.WithError(err).Error("visibilityForEvents: failed to fetch membership at event, defaulting to 'leave'")
		}
	}

	// Create a map from eventID -> eventVisibility
	for _, event := range events {
		eventID := event.EventID()
		vis := eventVisibility{
			membershipAtEvent: spec.Leave, // default to leave, to not expose events by accident
			visibility:        event.Visibility,
		}
		ev, ok := membershipEvents[eventID]
		if !ok || ev == nil {
			result[eventID] = vis
			continue
		}

		membership, err := ev.Membership()
		if err != nil {
			result[eventID] = vis
			continue
		}
		vis.membershipAtEvent = membership

		result[eventID] = vis
	}
	return result
}
