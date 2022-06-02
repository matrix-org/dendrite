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
)

type HistoryVisibility string

const (
	WorldReadable HistoryVisibility = "world_readable"
	Joined        HistoryVisibility = "joined"
	Shared        HistoryVisibility = "shared"
	Default       HistoryVisibility = "default"
	Invited       HistoryVisibility = "invited"
)

// EventVisibility contains the history visibility and membership state at a given event
type EventVisibility struct {
	Visibility         HistoryVisibility
	MembershipAtEvent  string
	MembershipCurrent  string
	MembershipPosition int // the topological position of the membership event
	HistoryPosition    int // the topological position of the history event
}

// Visibility is a map from event_id to EvVis, which contains the history visibility and membership for a given user.
type Visibility map[string]EventVisibility

// Allowed checks the Visibility map if the user is allowed to see the given event.
func (v Visibility) Allowed(eventID string) (allowed bool) {
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

// GetStateForEvents returns a Visibility map containing the state before and at the given events.
func GetStateForEvents(ctx context.Context, db storage.Database, events []gomatrixserverlib.ClientEvent, userID string) (Visibility, error) {
	result := make(map[string]EventVisibility, len(events))
	var (
		membershipCurrent string
		err               error
	)
	// try to get the current membership of the user
	if len(events) > 0 {
		membershipCurrent, _, err = db.SelectMembershipForUser(ctx, events[0].RoomID, userID, math.MaxInt64)
		if err != nil {
			return nil, err
		}
	}
	for _, ev := range events {
		// get the event topology position
		pos, err := db.EventPositionInTopology(ctx, ev.EventID)
		if err != nil {
			return nil, fmt.Errorf("initial event does not exist: %w", err)
		}
		// By default if no history_visibility is set, or if the value is not understood, the visibility is assumed to be shared
		var hisVis = "shared"
		historyEvent, historyPos, err := db.SelectTopologicalEvent(ctx, int(pos.Depth), "m.room.history_visibility", ev.RoomID)
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
		membership, memberPos, err := db.SelectMembershipForUser(ctx, ev.RoomID, userID, int64(pos.Depth))
		if err != nil {
			return nil, err
		}
		// finally create the mapping
		result[ev.EventID] = EventVisibility{
			Visibility:         HistoryVisibility(hisVis),
			MembershipAtEvent:  membership,
			MembershipCurrent:  membershipCurrent,
			MembershipPosition: memberPos,
			HistoryPosition:    int(historyPos.Depth),
		}
	}

	return result, nil
}
