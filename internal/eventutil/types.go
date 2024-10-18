// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package eventutil

import (
	"strconv"

	"github.com/element-hq/dendrite/syncapi/types"
)

// AccountData represents account data sent from the client API server to the
// sync API server
type AccountData struct {
	RoomID       string              `json:"room_id"`
	Type         string              `json:"type"`
	ReadMarker   *ReadMarkerJSON     `json:"read_marker,omitempty"`   // optional
	IgnoredUsers *types.IgnoredUsers `json:"ignored_users,omitempty"` // optional
}

type ReadMarkerJSON struct {
	FullyRead   string `json:"m.fully_read"`
	Read        string `json:"m.read"`
	ReadPrivate string `json:"m.read.private"`
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

// UserProfile is a struct containing all known user profile data
type UserProfile struct {
	AvatarURL   string `json:"avatar_url,omitempty"`
	DisplayName string `json:"displayname,omitempty"`
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
