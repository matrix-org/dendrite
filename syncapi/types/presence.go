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

package types

import (
	"strings"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

type Presence uint8

const (
	PresenceUnknown     Presence = iota
	PresenceUnavailable          // unavailable
	PresenceOnline               // online
	PresenceOffline              // offline
)

func (p Presence) String() string {
	switch p {
	case PresenceUnavailable:
		return "unavailable"
	case PresenceOnline:
		return "online"
	case PresenceOffline:
		return "offline"
	default:
		return "unknown"
	}
}

// PresenceFromString returns the integer representation of the given input presence.
// Returns false for ok, if input is not a valid presence value.
func PresenceFromString(input string) (Presence, bool) {
	switch strings.ToLower(input) {
	case "unavailable":
		return PresenceUnavailable, true
	case "online":
		return PresenceOnline, true
	case "offline":
		return PresenceOffline, true
	default:
		return PresenceUnknown, false
	}
}

type PresenceInternal struct {
	ClientFields PresenceClientResponse
	StreamPos    StreamPosition              `json:"-"`
	UserID       string                      `json:"-"`
	LastActiveTS gomatrixserverlib.Timestamp `json:"-"`
	Presence     Presence                    `json:"-"`
}

// Equals compares p1 with p2.
func (p1 *PresenceInternal) Equals(p2 *PresenceInternal) bool {
	return p1.ClientFields.Presence == p2.ClientFields.Presence &&
		p1.ClientFields.StatusMsg == p2.ClientFields.StatusMsg &&
		p1.UserID == p2.UserID
}

// CurrentlyActive returns the current active state.
func (p *PresenceInternal) CurrentlyActive() bool {
	return time.Since(p.LastActiveTS.Time()).Minutes() < 5
}

// LastActiveAgo returns the time since the LastActiveTS in milliseconds.
func (p *PresenceInternal) LastActiveAgo() int64 {
	return time.Since(p.LastActiveTS.Time()).Milliseconds()
}

type PresenceClientResponse struct {
	CurrentlyActive *bool   `json:"currently_active,omitempty"`
	LastActiveAgo   int64   `json:"last_active_ago,omitempty"`
	Presence        string  `json:"presence"`
	StatusMsg       *string `json:"status_msg,omitempty"`
}
