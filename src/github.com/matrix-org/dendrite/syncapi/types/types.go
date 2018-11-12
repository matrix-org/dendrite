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

package types

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/matrix-org/gomatrixserverlib"
)

var (
	// ErrInvalidPaginationTokenType is returned when an attempt at creating a
	// new instance of PaginationToken with an invalid type (i.e. neither "s"
	// nor "t").
	ErrInvalidPaginationTokenType = fmt.Errorf("Pagination token has an unknown prefix (should be either s or t)")
)

// StreamPosition represents the offset in the sync stream a client is at.
type StreamPosition int64

// String implements the Stringer interface.
func (sp StreamPosition) String() string {
	return strconv.FormatInt(int64(sp), 10)
}

// PaginationTokenType represents the type of a pagination token.
// It can be either "s" (representing a position in the whole stream of events)
// or "t" (representing a position in a room's topology/depth).
type PaginationTokenType string

const (
	// PaginationTokenTypeStream represents a position in the server's whole
	// stream of events
	PaginationTokenTypeStream PaginationTokenType = "s"
	// PaginationTokenTypeTopology represents a position in a room's topology.
	PaginationTokenTypeTopology PaginationTokenType = "t"
)

// PaginationToken represents a pagination token, used for interactions with
// /sync or /messages, for example.
type PaginationToken struct {
	Position StreamPosition
	Type     PaginationTokenType
}

// NewPaginationTokenFromString takes a string of the form "xyyyy..." where "x"
// represents the type of a pagination token and "yyyy..." the token itself, and
// parses it in order to create a new instance of PaginationToken. Returns an
// error if the token couldn't be parsed into an int64, or if the token type
// isn't a known type (returns ErrInvalidPaginationTokenType in the latter
// case).
func NewPaginationTokenFromString(s string) (p *PaginationToken, err error) {
	p = new(PaginationToken)

	// Parse the token (aka position).
	position, err := strconv.ParseInt(s[1:], 10, 64)
	if err != nil {
		return
	}
	p.Position = StreamPosition(position)

	// Check if the type is among the known ones.
	p.Type = PaginationTokenType(s[:1])
	if p.Type != PaginationTokenTypeStream && p.Type != PaginationTokenTypeTopology {
		err = ErrInvalidPaginationTokenType
	}

	return
}

// NewPaginationTokenFromTypeAndPosition takes a PaginationTokenType and a
// StreamPosition and returns an instance of PaginationToken.
func NewPaginationTokenFromTypeAndPosition(
	t PaginationTokenType, pos StreamPosition,
) (p *PaginationToken) {
	return &PaginationToken{
		Type:     t,
		Position: pos,
	}
}

// String translates a PaginationToken to a string of the "xyyyy..." (see
// NewPaginationToken to know what it represents).
func (p *PaginationToken) String() string {
	return fmt.Sprintf("%s%d", p.Type, p.Position)
}

// PrevEventRef represents a reference to a previous event in a state event upgrade
type PrevEventRef struct {
	PrevContent   json.RawMessage `json:"prev_content"`
	ReplacesState string          `json:"replaces_state"`
	PrevSender    string          `json:"prev_sender"`
}

// Response represents a /sync API response. See https://matrix.org/docs/spec/client_server/r0.2.0.html#get-matrix-client-r0-sync
type Response struct {
	NextBatch   string `json:"next_batch"`
	AccountData struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"account_data"`
	Presence struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"presence"`
	Rooms struct {
		Join   map[string]JoinResponse   `json:"join"`
		Invite map[string]InviteResponse `json:"invite"`
		Leave  map[string]LeaveResponse  `json:"leave"`
	} `json:"rooms"`
}

// NewResponse creates an empty response with initialised maps.
func NewResponse(pos StreamPosition) *Response {
	res := Response{}
	// Fill next_batch with a pagination token. Since this is a response to a sync request, we can assume
	// we'll always return a stream token.
	res.NextBatch = NewPaginationTokenFromTypeAndPosition(PaginationTokenTypeStream, pos).String()
	// Pre-initialise the maps. Synapse will return {} even if there are no rooms under a specific section,
	// so let's do the same thing. Bonus: this means we can't get dreaded 'assignment to entry in nil map' errors.
	res.Rooms.Join = make(map[string]JoinResponse)
	res.Rooms.Invite = make(map[string]InviteResponse)
	res.Rooms.Leave = make(map[string]LeaveResponse)

	// Also pre-intialise empty slices or else we'll insert 'null' instead of '[]' for the value.
	// TODO: We really shouldn't have to do all this to coerce encoding/json to Do The Right Thing. We should
	//       really be using our own Marshal/Unmarshal implementations otherwise this may prove to be a CPU bottleneck.
	//       This also applies to NewJoinResponse, NewInviteResponse and NewLeaveResponse.
	res.AccountData.Events = make([]gomatrixserverlib.ClientEvent, 0)
	res.Presence.Events = make([]gomatrixserverlib.ClientEvent, 0)

	return &res
}

// IsEmpty returns true if the response is empty, i.e. used to decided whether
// to return the response immediately to the client or to wait for more data.
func (r *Response) IsEmpty() bool {
	return len(r.Rooms.Join) == 0 &&
		len(r.Rooms.Invite) == 0 &&
		len(r.Rooms.Leave) == 0 &&
		len(r.AccountData.Events) == 0 &&
		len(r.Presence.Events) == 0
}

// JoinResponse represents a /sync response for a room which is under the 'join' key.
type JoinResponse struct {
	State struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"state"`
	Timeline struct {
		Events    []gomatrixserverlib.ClientEvent `json:"events"`
		Limited   bool                            `json:"limited"`
		PrevBatch string                          `json:"prev_batch"`
	} `json:"timeline"`
	Ephemeral struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"ephemeral"`
	AccountData struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"account_data"`
}

// NewJoinResponse creates an empty response with initialised arrays.
func NewJoinResponse() *JoinResponse {
	res := JoinResponse{}
	res.State.Events = make([]gomatrixserverlib.ClientEvent, 0)
	res.Timeline.Events = make([]gomatrixserverlib.ClientEvent, 0)
	res.Ephemeral.Events = make([]gomatrixserverlib.ClientEvent, 0)
	res.AccountData.Events = make([]gomatrixserverlib.ClientEvent, 0)
	return &res
}

// InviteResponse represents a /sync response for a room which is under the 'invite' key.
type InviteResponse struct {
	InviteState struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"invite_state"`
}

// NewInviteResponse creates an empty response with initialised arrays.
func NewInviteResponse() *InviteResponse {
	res := InviteResponse{}
	res.InviteState.Events = make([]gomatrixserverlib.ClientEvent, 0)
	return &res
}

// LeaveResponse represents a /sync response for a room which is under the 'leave' key.
type LeaveResponse struct {
	State struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"state"`
	Timeline struct {
		Events    []gomatrixserverlib.ClientEvent `json:"events"`
		Limited   bool                            `json:"limited"`
		PrevBatch string                          `json:"prev_batch"`
	} `json:"timeline"`
}

// NewLeaveResponse creates an empty response with initialised arrays.
func NewLeaveResponse() *LeaveResponse {
	res := LeaveResponse{}
	res.State.Events = make([]gomatrixserverlib.ClientEvent, 0)
	res.Timeline.Events = make([]gomatrixserverlib.ClientEvent, 0)
	return &res
}
