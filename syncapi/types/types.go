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
	"strconv"

	"github.com/matrix-org/gomatrixserverlib"
)

// SyncPosition contains the PDU and EDU stream sync positions for a client.
type SyncPosition struct {
	// PDUPosition is the stream position for PDUs the client is at.
	PDUPosition int64
	// TypingPosition is the position for typing notifications the client is at.
	TypingPosition int64
}

// String implements the Stringer interface.
func (sp SyncPosition) String() string {
	return strconv.FormatInt(sp.PDUPosition, 10) + "_" +
		strconv.FormatInt(sp.TypingPosition, 10)
}

// IsAfter returns whether sp refers to states newer than other
func (sp SyncPosition) IsAfter(other SyncPosition) bool {
	return sp.PDUPosition > other.PDUPosition ||
		sp.TypingPosition > other.TypingPosition
}

// WithUpdates returns a copy of `sp` with updates represented by `other` applied.
// If a field is not 0 in `other`, it is considered an update and its value
// will replace the corresponding value in sp.
func (sp SyncPosition) WithUpdates(other SyncPosition) SyncPosition {
	ret := sp
	if other.PDUPosition != 0 {
		ret.PDUPosition = other.PDUPosition
	}
	if other.TypingPosition != 0 {
		ret.TypingPosition = other.TypingPosition
	}
	return ret
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
func NewResponse(pos SyncPosition) *Response {
	res := Response{
		NextBatch: pos.String(),
	}
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
