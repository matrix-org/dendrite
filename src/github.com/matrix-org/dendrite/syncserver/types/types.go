package types

import (
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/events"
	"github.com/matrix-org/gomatrixserverlib"
)

// StreamPosition represents the offset in the sync stream a client is at.
type StreamPosition int64

// String implements the Stringer interface.
func (sp StreamPosition) String() string {
	return strconv.FormatInt(int64(sp), 10)
}

// RoomData represents the data for a room suitable for building a sync response from.
type RoomData struct {
	State        []gomatrixserverlib.Event
	RecentEvents []gomatrixserverlib.Event
}

// Response represents a /sync API response. See https://matrix.org/docs/spec/client_server/r0.2.0.html#get-matrix-client-r0-sync
type Response struct {
	NextBatch   string `json:"next_batch"`
	AccountData struct {
		Events []events.ClientEvent `json:"events"`
	} `json:"account_data"`
	Presence struct {
		Events []events.ClientEvent `json:"events"`
	} `json:"presence"`
	Rooms struct {
		Join   map[string]JoinResponse   `json:"join"`
		Invite map[string]InviteResponse `json:"invite"`
		Leave  map[string]LeaveResponse  `json:"leave"`
	} `json:"rooms"`
}

// NewResponse creates an empty response with initialised maps.
func NewResponse() *Response {
	res := Response{}
	// Pre-initalise the maps. Synapse will return {} even if there are no rooms under a specific section,
	// so let's do the same thing. Bonus: this means we can't get dreaded 'assignment to entry in nil map' errors.
	res.Rooms.Join = make(map[string]JoinResponse)
	res.Rooms.Invite = make(map[string]InviteResponse)
	res.Rooms.Leave = make(map[string]LeaveResponse)

	// Also pre-intialise empty slices or else we'll insert 'null' instead of '[]' for the value.
	// TODO: We really shouldn't have to do all this to coerce encoding/json to Do The Right Thing. We should
	//       really be using our own Marshal/Unmarshal implementations otherwise this may prove to be a CPU bottleneck.
	//       This also applies to NewJoinResponse, NewInviteResponse and NewLeaveResponse.
	res.AccountData.Events = make([]events.ClientEvent, 0)
	res.Presence.Events = make([]events.ClientEvent, 0)

	return &res
}

// JoinResponse represents a /sync response for a room which is under the 'join' key.
type JoinResponse struct {
	State struct {
		Events []events.ClientEvent `json:"events"`
	} `json:"state"`
	Timeline struct {
		Events    []events.ClientEvent `json:"events"`
		Limited   bool                 `json:"limited"`
		PrevBatch string               `json:"prev_batch"`
	} `json:"timeline"`
	Ephemeral struct {
		Events []events.ClientEvent `json:"events"`
	} `json:"ephemeral"`
	AccountData struct {
		Events []events.ClientEvent `json:"events"`
	} `json:"account_data"`
}

// NewJoinResponse creates an empty response with initialised arrays.
func NewJoinResponse() *JoinResponse {
	res := JoinResponse{}
	res.State.Events = make([]events.ClientEvent, 0)
	res.Timeline.Events = make([]events.ClientEvent, 0)
	res.Ephemeral.Events = make([]events.ClientEvent, 0)
	res.AccountData.Events = make([]events.ClientEvent, 0)
	return &res
}

// InviteResponse represents a /sync response for a room which is under the 'invite' key.
type InviteResponse struct {
	InviteState struct {
		Events []events.ClientEvent
	} `json:"invite_state"`
}

// NewInviteResponse creates an empty response with initialised arrays.
func NewInviteResponse() *InviteResponse {
	res := InviteResponse{}
	res.InviteState.Events = make([]events.ClientEvent, 0)
	return &res
}

// LeaveResponse represents a /sync response for a room which is under the 'leave' key.
type LeaveResponse struct {
	State struct {
		Events []events.ClientEvent `json:"events"`
	} `json:"state"`
	Timeline struct {
		Events    []events.ClientEvent `json:"events"`
		Limited   bool                 `json:"limited"`
		PrevBatch string               `json:"prev_batch"`
	} `json:"timeline"`
}

// NewLeaveResponse creates an empty response with initialised arrays.
func NewLeaveResponse() *LeaveResponse {
	res := LeaveResponse{}
	res.State.Events = make([]events.ClientEvent, 0)
	res.Timeline.Events = make([]events.ClientEvent, 0)
	return &res
}
