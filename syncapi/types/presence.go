// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package types

import (
	"strings"
	"time"

	"github.com/matrix-org/gomatrixserverlib/spec"
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
	StreamPos    StreamPosition `json:"-"`
	UserID       string         `json:"-"`
	LastActiveTS spec.Timestamp `json:"-"`
	Presence     Presence       `json:"-"`
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
