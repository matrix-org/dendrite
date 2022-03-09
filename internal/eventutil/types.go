// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package eventutil

import (
	"errors"
	"strconv"
)

// ErrProfileNoExists is returned when trying to lookup a user's profile that
// doesn't exist locally.
var ErrProfileNoExists = errors.New("no known profile for given user ID")

// AccountData represents account data sent from the client API server to the
// sync API server
type AccountData struct {
	RoomID     string          `json:"room_id"`
	Type       string          `json:"type"`
	ReadMarker *ReadMarkerJSON `json:"read_marker,omitempty"` // optional
}

type ReadMarkerJSON struct {
	FullyRead string `json:"m.fully_read"`
	Read      string `json:"m.read"`
}

// NotificationData contains statistics about notifications, sent from
// the Push Server to the Sync API server.
type NotificationData struct {
	// RoomID identifies the scope of the statistics, together with
	// MXID (which is encoded in the Kafka key).
	RoomID string `json:"room_id"`

	// HighlightCount is the number of unread notifications with the
	// highlight tweak.
	UnreadHighlightCount int `json:"unread_highlight_count"`

	// UnreadNotificationCount is the total number of unread
	// notifications.
	UnreadNotificationCount int `json:"unread_notification_count"`
}

// ProfileResponse is a struct containing all known user profile data
type ProfileResponse struct {
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"displayname"`
}

// AvatarURL is a struct containing only the URL to a user's avatar
type AvatarURL struct {
	AvatarURL string `json:"avatar_url"`
}

// DisplayName is a struct containing only a user's display name
type DisplayName struct {
	DisplayName string `json:"displayname"`
}

// WeakBoolean is a type that will Unmarshal to true or false even if the encoded
// representation is "true"/1 or "false"/0, as well as whatever other forms are
// recognised by strconv.ParseBool
type WeakBoolean bool

// UnmarshalJSON is overridden here to allow strings vaguely representing a true
// or false boolean to be set as their closest counterpart
func (b *WeakBoolean) UnmarshalJSON(data []byte) error {
	result, err := strconv.ParseBool(string(data))
	if err != nil {
		return err
	}

	// Set boolean value based on string input
	*b = WeakBoolean(result)

	return nil
}
