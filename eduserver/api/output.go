// Copyright 2017 Vector Creations Ltd
// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package api

import (
	"time"

	types2 "github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/userapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// OutputTypingEvent is an entry in typing server output kafka log.
// This contains the event with extra fields used to create 'm.typing' event
// in clientapi & federation.
type OutputTypingEvent struct {
	// The Event for the typing edu event.
	Event TypingEvent `json:"event"`
	// ExpireTime is the interval after which the user should no longer be
	// considered typing. Only available if Event.Typing is true.
	ExpireTime *time.Time
}

// OutputSendToDeviceEvent is an entry in the send-to-device output kafka log.
// This contains the full event content, along with the user ID and device ID
// to which it is destined.
type OutputSendToDeviceEvent struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	gomatrixserverlib.SendToDeviceEvent
}

// OutputReceiptEvent is an entry in the receipt output kafka log
type OutputReceiptEvent struct {
	UserID    string                      `json:"user_id"`
	RoomID    string                      `json:"room_id"`
	EventID   string                      `json:"event_id"`
	Type      string                      `json:"type"`
	Timestamp gomatrixserverlib.Timestamp `json:"timestamp"`
}

// OutputCrossSigningKeyUpdate is an entry in the signing key update output kafka log
type OutputCrossSigningKeyUpdate struct {
	CrossSigningKeyUpdate `json:"signing_keys"`
}

// OutputPresenceData is an entry in the presence output kafka log
type OutputPresenceData struct {
	UserID        string                      `json:"user_id"`
	Presence      types.PresenceStatus        `json:"presence"`
	StatusMsg     *string                     `json:"status_msg,omitempty"`
	LastActiveTS  gomatrixserverlib.Timestamp `json:"last_active_ts"`
	LastActiveAgo int64                       `json:"last_active_ago,omitempty"`
	StreamPos     types2.StreamPosition       `json:"stream_pos"`
}

type FederationPresenceSingle struct {
	CurrentlyActive bool    `json:"currently_active"`
	LastActiveAgo   int     `json:"last_active_ago"`
	Presence        string  `json:"presence"`
	UserID          string  `json:"user_id"`
	StatusMsg       *string `json:"status_msg"`
}

type FederationPresenceData struct {
	Push []FederationPresenceSingle `json:"push"`
}
