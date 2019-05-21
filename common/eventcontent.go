// Copyright 2017 Vector Creations Ltd
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

package common

// CreateContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-create
type CreateContent struct {
	Creator  string `json:"creator"`
	Federate *bool  `json:"m.federate,omitempty"`
}

// MemberContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-member
type MemberContent struct {
	Membership       string    `json:"membership"`
	DisplayName      string    `json:"displayname,omitempty"`
	AvatarURL        string    `json:"avatar_url,omitempty"`
	Reason           string    `json:"reason,omitempty"`
	ThirdPartyInvite *TPInvite `json:"third_party_invite,omitempty"`
}

// TPInvite is the "Invite" structure defined at http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-member
type TPInvite struct {
	DisplayName string         `json:"display_name"`
	Signed      TPInviteSigned `json:"signed"`
}

// TPInviteSigned is the "signed" structure defined at http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-member
type TPInviteSigned struct {
	MXID       string                       `json:"mxid"`
	Signatures map[string]map[string]string `json:"signatures"`
	Token      string                       `json:"token"`
}

// ThirdPartyInviteContent is the content event for https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-third-party-invite
type ThirdPartyInviteContent struct {
	DisplayName    string      `json:"display_name"`
	KeyValidityURL string      `json:"key_validity_url"`
	PublicKey      string      `json:"public_key"`
	PublicKeys     []PublicKey `json:"public_keys"`
}

// PublicKey is the PublicKeys structure in https://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-third-party-invite
type PublicKey struct {
	KeyValidityURL string `json:"key_validity_url"`
	PublicKey      string `json:"public_key"`
}

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

// JoinRulesContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-join-rules
type JoinRulesContent struct {
	JoinRule string `json:"join_rule"`
}

// HistoryVisibilityContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-history-visibility
type HistoryVisibilityContent struct {
	HistoryVisibility string `json:"history_visibility"`
}

// PowerLevelContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-power-levels
type PowerLevelContent struct {
	EventsDefault int            `json:"events_default"`
	Invite        int            `json:"invite"`
	StateDefault  int            `json:"state_default"`
	Redact        int            `json:"redact"`
	Ban           int            `json:"ban"`
	UsersDefault  int            `json:"users_default"`
	Events        map[string]int `json:"events"`
	Kick          int            `json:"kick"`
	Users         map[string]int `json:"users"`
}

// InitialPowerLevelsContent returns the initial values for m.room.power_levels on room creation
// if they have not been specified.
// http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-power-levels
// https://github.com/matrix-org/synapse/blob/v0.19.2/synapse/handlers/room.py#L294
func InitialPowerLevelsContent(roomCreator string) PowerLevelContent {
	return PowerLevelContent{
		EventsDefault: 0,
		Invite:        0,
		StateDefault:  50,
		Redact:        50,
		Ban:           50,
		UsersDefault:  0,
		Events: map[string]int{
			"m.room.name":               50,
			"m.room.power_levels":       100,
			"m.room.history_visibility": 100,
			"m.room.canonical_alias":    50,
			"m.room.avatar":             50,
		},
		Kick:  50,
		Users: map[string]int{roomCreator: 100},
	}
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
