package common

// CreateContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-create
type CreateContent struct {
	Creator  string `json:"creator"`
	Federate *bool  `json:"m.federate,omitempty"`
}

// MemberContent is the event content for http://matrix.org/docs/spec/client_server/r0.2.0.html#m-room-member
type MemberContent struct {
	Membership  string `json:"membership"`
	DisplayName string `json:"displayname,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	// TODO: ThirdPartyInvite string `json:"third_party_invite,omitempty"`
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
