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
	"sort"
	"strconv"
	"strings"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
)

var (
	// ErrInvalidSyncTokenType is returned when an attempt at creating a
	// new instance of SyncToken with an invalid type (i.e. neither "s"
	// nor "t").
	ErrInvalidSyncTokenType = fmt.Errorf("Sync token has an unknown prefix (should be either s or t)")
	// ErrInvalidSyncTokenLen is returned when the pagination token is an
	// invalid length
	ErrInvalidSyncTokenLen = fmt.Errorf("Sync token has an invalid length")
)

// StreamPosition represents the offset in the sync stream a client is at.
type StreamPosition int64

// LogPosition represents the offset in a Kafka log a client is at.
type LogPosition struct {
	Partition int32
	Offset    int64
}

// IsAfter returns true if this position is after `lp`.
func (p *LogPosition) IsAfter(lp *LogPosition) bool {
	if lp == nil {
		return false
	}
	if p.Partition != lp.Partition {
		return false
	}
	return p.Offset > lp.Offset
}

// StreamEvent is the same as gomatrixserverlib.Event but also has the PDU stream position for this event.
type StreamEvent struct {
	*gomatrixserverlib.HeaderedEvent
	StreamPosition  StreamPosition
	TransactionID   *api.TransactionID
	ExcludeFromSync bool
}

// Range represents a range between two stream positions.
type Range struct {
	// From is the position the client has already received.
	From StreamPosition
	// To is the position the client is going towards.
	To StreamPosition
	// True if the client is going backwards
	Backwards bool
}

// Low returns the low number of the range.
// This represents the position the client already has and hence is exclusive.
func (r *Range) Low() StreamPosition {
	if !r.Backwards {
		return r.From
	}
	return r.To
}

// High returns the high number of the range
// This represents the position the client is going towards and hence is inclusive.
func (r *Range) High() StreamPosition {
	if !r.Backwards {
		return r.To
	}
	return r.From
}

// SyncTokenType represents the type of a sync token.
// It can be either "s" (representing a position in the whole stream of events)
// or "t" (representing a position in a room's topology/depth).
type SyncTokenType string

const (
	// SyncTokenTypeStream represents a position in the server's whole
	// stream of events
	SyncTokenTypeStream SyncTokenType = "s"
	// SyncTokenTypeTopology represents a position in a room's topology.
	SyncTokenTypeTopology SyncTokenType = "t"
)

type StreamingToken struct {
	PDUPosition          StreamPosition
	TypingPosition       StreamPosition
	ReceiptPosition      StreamPosition
	SendToDevicePosition StreamPosition
	Logs                 map[string]*LogPosition
}

func (t *StreamingToken) SetLog(name string, lp *LogPosition) {
	if t.Logs == nil {
		t.Logs = make(map[string]*LogPosition)
	}
	t.Logs[name] = lp
}

func (t *StreamingToken) Log(name string) *LogPosition {
	l, ok := t.Logs[name]
	if !ok {
		return nil
	}
	return l
}

func (t StreamingToken) String() string {
	posStr := fmt.Sprintf(
		"s%d_%d_%d_%d",
		t.PDUPosition, t.TypingPosition,
		t.ReceiptPosition, t.SendToDevicePosition,
	)
	var logStrings []string
	for name, lp := range t.Logs {
		logStr := fmt.Sprintf("%s-%d-%d", name, lp.Partition, lp.Offset)
		logStrings = append(logStrings, logStr)
	}
	sort.Strings(logStrings)
	// E.g s11_22_33_44.dl0-134.ab1-441
	return strings.Join(append([]string{posStr}, logStrings...), ".")
}

// IsAfter returns true if ANY position in this token is greater than `other`.
func (t *StreamingToken) IsAfter(other StreamingToken) bool {
	switch {
	case t.PDUPosition > other.PDUPosition:
		return true
	case t.TypingPosition > other.TypingPosition:
		return true
	case t.ReceiptPosition > other.ReceiptPosition:
		return true
	case t.SendToDevicePosition > other.SendToDevicePosition:
		return true
	}
	for name := range t.Logs {
		otherLog := other.Log(name)
		if otherLog == nil {
			continue
		}
		if t.Logs[name].IsAfter(otherLog) {
			return true
		}
	}
	return false
}

func (t *StreamingToken) IsEmpty() bool {
	return t == nil || t.PDUPosition+t.TypingPosition+t.ReceiptPosition+t.SendToDevicePosition == 0
}

// WithUpdates returns a copy of the StreamingToken with updates applied from another StreamingToken.
// If the latter StreamingToken contains a field that is not 0, it is considered an update,
// and its value will replace the corresponding value in the StreamingToken on which WithUpdates is called.
// If the other token has a log, they will replace any existing log on this token.
func (t *StreamingToken) WithUpdates(other StreamingToken) (ret StreamingToken) {
	ret = *t
	switch {
	case other.PDUPosition > 0:
		ret.PDUPosition = other.PDUPosition
	case other.TypingPosition > 0:
		ret.TypingPosition = other.TypingPosition
	case other.ReceiptPosition > 0:
		ret.ReceiptPosition = other.ReceiptPosition
	case other.SendToDevicePosition > 0:
		ret.SendToDevicePosition = other.SendToDevicePosition
	}
	ret.Logs = make(map[string]*LogPosition)
	for name := range t.Logs {
		otherLog := other.Log(name)
		if otherLog == nil {
			continue
		}
		copy := *otherLog
		ret.Logs[name] = &copy
	}
	return ret
}

type TopologyToken struct {
	Depth       StreamPosition
	PDUPosition StreamPosition
}

func (t *TopologyToken) StreamToken() StreamingToken {
	return StreamingToken{
		PDUPosition: t.PDUPosition,
	}
}

func (t TopologyToken) String() string {
	return fmt.Sprintf("t%d_%d", t.Depth, t.PDUPosition)
}

// Decrement the topology token to one event earlier.
func (t *TopologyToken) Decrement() {
	depth := t.Depth
	pduPos := t.PDUPosition
	if depth-1 <= 0 {
		// nothing can be lower than this
		depth = 1
	} else {
		// this assumes that we will never have 1000 events all with the same
		// depth. TODO: work out what the right PDU position is to use, probably needs a db hit.
		depth--
		pduPos += 1000
	}
	// The lowest token value is 1, therefore we need to manually set it to that
	// value if we're below it.
	if depth < 1 {
		depth = 1
	}
	t.Depth = depth
	t.PDUPosition = pduPos
}

func NewTopologyTokenFromString(tok string) (token TopologyToken, err error) {
	if len(tok) < 1 {
		err = fmt.Errorf("empty topology token")
		return
	}
	if tok[0] != SyncTokenTypeTopology[0] {
		err = fmt.Errorf("topology token must start with 't'")
		return
	}
	parts := strings.Split(tok[1:], "_")
	var positions [2]StreamPosition
	for i, p := range parts {
		if i > len(positions) {
			break
		}
		var pos int
		pos, err = strconv.Atoi(p)
		if err != nil {
			return
		}
		positions[i] = StreamPosition(pos)
	}
	token = TopologyToken{
		Depth:       positions[0],
		PDUPosition: positions[1],
	}
	return
}

func NewStreamTokenFromString(tok string) (token StreamingToken, err error) {
	if len(tok) < 1 {
		err = fmt.Errorf("empty stream token")
		return
	}
	if tok[0] != SyncTokenTypeStream[0] {
		err = fmt.Errorf("stream token must start with 's'")
		return
	}
	categories := strings.Split(tok[1:], ".")
	parts := strings.Split(categories[0], "_")
	var positions [4]StreamPosition
	for i, p := range parts {
		if i > len(positions) {
			break
		}
		var pos int
		pos, err = strconv.Atoi(p)
		if err != nil {
			return
		}
		positions[i] = StreamPosition(pos)
	}
	token = StreamingToken{
		PDUPosition:          positions[0],
		TypingPosition:       positions[1],
		ReceiptPosition:      positions[2],
		SendToDevicePosition: positions[3],
		Logs:                 make(map[string]*LogPosition),
	}
	// dl-0-1234
	// $log_name-$partition-$offset
	for _, logStr := range categories[1:] {
		segments := strings.Split(logStr, "-")
		if len(segments) != 3 {
			err = fmt.Errorf("token %s - invalid log: %s", tok, logStr)
			return
		}
		var partition int64
		partition, err = strconv.ParseInt(segments[1], 10, 32)
		if err != nil {
			return
		}
		var offset int64
		offset, err = strconv.ParseInt(segments[2], 10, 64)
		if err != nil {
			return
		}
		token.Logs[segments[0]] = &LogPosition{
			Partition: int32(partition),
			Offset:    offset,
		}
	}
	return token, nil
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
	} `json:"account_data,omitempty"`
	Presence struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"presence,omitempty"`
	Rooms struct {
		Join   map[string]JoinResponse   `json:"join"`
		Peek   map[string]JoinResponse   `json:"peek"`
		Invite map[string]InviteResponse `json:"invite"`
		Leave  map[string]LeaveResponse  `json:"leave"`
	} `json:"rooms"`
	ToDevice struct {
		Events []gomatrixserverlib.SendToDeviceEvent `json:"events"`
	} `json:"to_device"`
	DeviceLists struct {
		Changed []string `json:"changed,omitempty"`
		Left    []string `json:"left,omitempty"`
	} `json:"device_lists,omitempty"`
	DeviceListsOTKCount map[string]int `json:"device_one_time_keys_count"`
}

// NewResponse creates an empty response with initialised maps.
func NewResponse() *Response {
	res := Response{}
	// Pre-initialise the maps. Synapse will return {} even if there are no rooms under a specific section,
	// so let's do the same thing. Bonus: this means we can't get dreaded 'assignment to entry in nil map' errors.
	res.Rooms.Join = make(map[string]JoinResponse)
	res.Rooms.Peek = make(map[string]JoinResponse)
	res.Rooms.Invite = make(map[string]InviteResponse)
	res.Rooms.Leave = make(map[string]LeaveResponse)

	// Also pre-intialise empty slices or else we'll insert 'null' instead of '[]' for the value.
	// TODO: We really shouldn't have to do all this to coerce encoding/json to Do The Right Thing. We should
	//       really be using our own Marshal/Unmarshal implementations otherwise this may prove to be a CPU bottleneck.
	//       This also applies to NewJoinResponse, NewInviteResponse and NewLeaveResponse.
	res.AccountData.Events = make([]gomatrixserverlib.ClientEvent, 0)
	res.Presence.Events = make([]gomatrixserverlib.ClientEvent, 0)
	res.ToDevice.Events = make([]gomatrixserverlib.SendToDeviceEvent, 0)
	res.DeviceListsOTKCount = make(map[string]int)

	return &res
}

// IsEmpty returns true if the response is empty, i.e. used to decided whether
// to return the response immediately to the client or to wait for more data.
func (r *Response) IsEmpty() bool {
	return len(r.Rooms.Join) == 0 &&
		len(r.Rooms.Invite) == 0 &&
		len(r.Rooms.Leave) == 0 &&
		len(r.AccountData.Events) == 0 &&
		len(r.Presence.Events) == 0 &&
		len(r.ToDevice.Events) == 0
}

// JoinResponse represents a /sync response for a room which is under the 'join' or 'peek' key.
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
		Events []json.RawMessage `json:"events"`
	} `json:"invite_state"`
}

// NewInviteResponse creates an empty response with initialised arrays.
func NewInviteResponse(event *gomatrixserverlib.HeaderedEvent) *InviteResponse {
	res := InviteResponse{}
	res.InviteState.Events = []json.RawMessage{}

	// First see if there's invite_room_state in the unsigned key of the invite.
	// If there is then unmarshal it into the response. This will contain the
	// partial room state such as join rules, room name etc.
	if inviteRoomState := gjson.GetBytes(event.Unsigned(), "invite_room_state"); inviteRoomState.Exists() {
		_ = json.Unmarshal([]byte(inviteRoomState.Raw), &res.InviteState.Events)
	}

	// Then we'll see if we can create a partial of the invite event itself.
	// This is needed for clients to work out *who* sent the invite.
	inviteEvent := gomatrixserverlib.ToClientEvent(event.Unwrap(), gomatrixserverlib.FormatSync)
	inviteEvent.Unsigned = nil
	if ev, err := json.Marshal(inviteEvent); err == nil {
		res.InviteState.Events = append(res.InviteState.Events, ev)
	}

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

type SendToDeviceNID int

type SendToDeviceEvent struct {
	gomatrixserverlib.SendToDeviceEvent
	ID          SendToDeviceNID
	UserID      string
	DeviceID    string
	SentByToken *StreamingToken
}

type PeekingDevice struct {
	UserID   string
	DeviceID string
}

type Peek struct {
	RoomID  string
	New     bool
	Deleted bool
}
