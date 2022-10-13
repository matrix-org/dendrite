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

package auth

import (
	"github.com/matrix-org/gomatrixserverlib"
)

// TODO: This logic should live in gomatrixserverlib

// IsServerAllowed returns true if the server is allowed to see events in the room
// at this particular state. This function implements https://matrix.org/docs/spec/client_server/r0.6.0#id87
func IsServerAllowed(
	serverName gomatrixserverlib.ServerName,
	serverCurrentlyInRoom bool,
	authEvents []*gomatrixserverlib.Event,
) bool {
	historyVisibility := HistoryVisibilityForRoom(authEvents)

	// 1. If the history_visibility was set to world_readable, allow.
	if historyVisibility == gomatrixserverlib.HistoryVisibilityWorldReadable {
		return true
	}
	// 2. If the user's membership was join, allow.
	joinedUserExists := IsAnyUserOnServerWithMembership(serverName, authEvents, gomatrixserverlib.Join)
	if joinedUserExists {
		return true
	}
	// 3. If history_visibility was set to shared, and the user joined the room at any point after the event was sent, allow.
	if historyVisibility == gomatrixserverlib.HistoryVisibilityShared && serverCurrentlyInRoom {
		return true
	}
	// 4. If the user's membership was invite, and the history_visibility was set to invited, allow.
	invitedUserExists := IsAnyUserOnServerWithMembership(serverName, authEvents, gomatrixserverlib.Invite)
	if invitedUserExists && historyVisibility == gomatrixserverlib.HistoryVisibilityInvited {
		return true
	}

	// 5. Otherwise, deny.
	return false
}

func HistoryVisibilityForRoom(authEvents []*gomatrixserverlib.Event) gomatrixserverlib.HistoryVisibility {
	// https://matrix.org/docs/spec/client_server/r0.6.0#id87
	// By default if no history_visibility is set, or if the value is not understood, the visibility is assumed to be shared.
	visibility := gomatrixserverlib.HistoryVisibilityShared
	for _, ev := range authEvents {
		if ev.Type() != gomatrixserverlib.MRoomHistoryVisibility {
			continue
		}
		if vis, err := ev.HistoryVisibility(); err == nil {
			visibility = vis
		}
	}
	return visibility
}

func IsAnyUserOnServerWithMembership(serverName gomatrixserverlib.ServerName, authEvents []*gomatrixserverlib.Event, wantMembership string) bool {
	for _, ev := range authEvents {
		if ev.Type() != gomatrixserverlib.MRoomMember {
			continue
		}
		membership, err := ev.Membership()
		if err != nil || membership != wantMembership {
			continue
		}

		stateKey := ev.StateKey()
		if stateKey == nil {
			continue
		}

		_, domain, err := gomatrixserverlib.SplitID('@', *stateKey)
		if err != nil {
			continue
		}

		if domain == serverName {
			return true
		}
	}
	return false
}
