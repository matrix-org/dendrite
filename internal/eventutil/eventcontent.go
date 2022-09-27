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

import "github.com/matrix-org/gomatrixserverlib"

// NameContent is the event content for https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-name
type NameContent struct {
	Name string `json:"name"`
}

// TopicContent is the event content for https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-topic
type TopicContent struct {
	Topic string `json:"topic"`
}

// GuestAccessContent is the event content for https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-guest-access
type GuestAccessContent struct {
	GuestAccess string `json:"guest_access"`
}

// HistoryVisibilityContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-history-visibility
type HistoryVisibilityContent struct {
	HistoryVisibility string `json:"history_visibility"`
}

// CanonicalAlias is the event content for https://matrix.org/docs/spec/client_server/r0.6.0#m-room-canonical-alias
type CanonicalAlias struct {
	Alias string `json:"alias"`
}

// InitialPowerLevelsContent returns the initial values for m.room.power_levels on room creation
// if they have not been specified.
// http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-power-levels
// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/handlers/room.py#L294
func InitialPowerLevelsContent(roomCreator string) (c gomatrixserverlib.PowerLevelContent) {
	c.Defaults()
	c.Events = map[string]int64{
		"m.room.name":               50,
		"m.room.power_levels":       100,
		"m.room.history_visibility": 100,
		"m.room.canonical_alias":    50,
		"m.room.avatar":             50,
		"m.room.tombstone":          100,
		"m.room.encryption":         100,
		"m.room.server_acl":         100,
	}
	c.Users = map[string]int64{roomCreator: 100}
	return c
}

// AliasesContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-aliases
type AliasesContent struct {
	Aliases []string `json:"aliases"`
}

// CanonicalAliasContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-canonical-alias
type CanonicalAliasContent struct {
	Alias string `json:"alias"`
}

// AvatarContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-avatar
type AvatarContent struct {
	Info          ImageInfo `json:"info,omitempty"`
	URL           string    `json:"url"`
	ThumbnailURL  string    `json:"thumbnail_url,omitempty"`
	ThumbnailInfo ImageInfo `json:"thumbnail_info,omitempty"`
}

// ImageInfo implements the ImageInfo structure from http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-avatar
type ImageInfo struct {
	Mimetype string `json:"mimetype"`
	Height   int64  `json:"h"`
	Width    int64  `json:"w"`
	Size     int64  `json:"size"`
}
