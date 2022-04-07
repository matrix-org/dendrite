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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
)

var (
	// This error is returned when parsing sync tokens if the token is invalid. Callers can use this
	// error to detect whether to 400 or 401 the client. It is recommended to 401 them to force a
	// logout.
	ErrMalformedSyncToken = errors.New("malformed sync token")
)

type StateDelta struct {
	RoomID      string
	StateEvents []*gomatrixserverlib.HeaderedEvent
	Membership  string
	// The PDU stream position of the latest membership event for this user, if applicable.
	// Can be 0 if there is no membership event in this delta.
	MembershipPos StreamPosition
}

// StreamPosition represents the offset in the sync stream a client is at.
type StreamPosition int64

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
	PDUPosition              StreamPosition
	TypingPosition           StreamPosition
	ReceiptPosition          StreamPosition
	SendToDevicePosition     StreamPosition
	InvitePosition           StreamPosition
	AccountDataPosition      StreamPosition
	DeviceListPosition       StreamPosition
	NotificationDataPosition StreamPosition
}

// This will be used as a fallback by json.Marshal.
func (s StreamingToken) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// This will be used as a fallback by json.Unmarshal.
func (s *StreamingToken) UnmarshalText(text []byte) (err error) {
	*s, err = NewStreamTokenFromString(string(text))
	return err
}

func (t StreamingToken) String() string {
	posStr := fmt.Sprintf(
		"s%d_%d_%d_%d_%d_%d_%d_%d",
		t.PDUPosition, t.TypingPosition,
		t.ReceiptPosition, t.SendToDevicePosition,
		t.InvitePosition, t.AccountDataPosition,
		t.DeviceListPosition, t.NotificationDataPosition,
	)
	return posStr
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
	case t.InvitePosition > other.InvitePosition:
		return true
	case t.AccountDataPosition > other.AccountDataPosition:
		return true
	case t.DeviceListPosition > other.DeviceListPosition:
		return true
	case t.NotificationDataPosition > other.NotificationDataPosition:
		return true
	}
	return false
}

func (t *StreamingToken) IsEmpty() bool {
	return t == nil || t.PDUPosition+t.TypingPosition+t.ReceiptPosition+t.SendToDevicePosition+t.InvitePosition+t.AccountDataPosition+t.DeviceListPosition+t.NotificationDataPosition == 0
}

// WithUpdates returns a copy of the StreamingToken with updates applied from another StreamingToken.
// If the latter StreamingToken contains a field that is not 0, it is considered an update,
// and its value will replace the corresponding value in the StreamingToken on which WithUpdates is called.
// If the other token has a log, they will replace any existing log on this token.
func (t *StreamingToken) WithUpdates(other StreamingToken) StreamingToken {
	ret := *t
	ret.ApplyUpdates(other)
	return ret
}

// ApplyUpdates applies any changes from the supplied StreamingToken. If the supplied
// streaming token contains any positions that are not 0, they are considered updates
// and will overwrite the value in the token.
func (t *StreamingToken) ApplyUpdates(other StreamingToken) {
	if other.PDUPosition > t.PDUPosition {
		t.PDUPosition = other.PDUPosition
	}
	if other.TypingPosition > t.TypingPosition {
		t.TypingPosition = other.TypingPosition
	}
	if other.ReceiptPosition > t.ReceiptPosition {
		t.ReceiptPosition = other.ReceiptPosition
	}
	if other.SendToDevicePosition > t.SendToDevicePosition {
		t.SendToDevicePosition = other.SendToDevicePosition
	}
	if other.InvitePosition > t.InvitePosition {
		t.InvitePosition = other.InvitePosition
	}
	if other.AccountDataPosition > t.AccountDataPosition {
		t.AccountDataPosition = other.AccountDataPosition
	}
	if other.DeviceListPosition > t.DeviceListPosition {
		t.DeviceListPosition = other.DeviceListPosition
	}
	if other.NotificationDataPosition > t.NotificationDataPosition {
		t.NotificationDataPosition = other.NotificationDataPosition
	}
}

type TopologyToken struct {
	Depth       StreamPosition
	PDUPosition StreamPosition
}

// This will be used as a fallback by json.Marshal.
func (t TopologyToken) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// This will be used as a fallback by json.Unmarshal.
func (t *TopologyToken) UnmarshalText(text []byte) (err error) {
	*t, err = NewTopologyTokenFromString(string(text))
	return err
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
		err = ErrMalformedSyncToken
		return
	}
	if tok[0] != SyncTokenTypeStream[0] {
		err = ErrMalformedSyncToken
		return
	}
	// Migration: Remove everything after and including '.' - we previously had tokens like:
	// s478_0_0_0_0_13.dl-0-2 but we have now removed partitioned stream positions
	tok = strings.Split(tok, ".")[0]
	parts := strings.Split(tok[1:], "_")
	var positions [8]StreamPosition
	for i, p := range parts {
		if i >= len(positions) {
			break
		}
		var pos int
		pos, err = strconv.Atoi(p)
		if err != nil {
			err = ErrMalformedSyncToken
			return
		}
		positions[i] = StreamPosition(pos)
	}
	token = StreamingToken{
		PDUPosition:              positions[0],
		TypingPosition:           positions[1],
		ReceiptPosition:          positions[2],
		SendToDevicePosition:     positions[3],
		InvitePosition:           positions[4],
		AccountDataPosition:      positions[5],
		DeviceListPosition:       positions[6],
		NotificationDataPosition: positions[7],
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
	NextBatch   StreamingToken `json:"next_batch"`
	AccountData struct {
		Events []gomatrixserverlib.ClientEvent `json:"events,omitempty"`
	} `json:"account_data"`
	Presence struct {
		Events []gomatrixserverlib.ClientEvent `json:"events,omitempty"`
	} `json:"presence"`
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
	} `json:"device_lists"`
	DeviceListsOTKCount map[string]int `json:"device_one_time_keys_count,omitempty"`
}

// NewResponse creates an empty response with initialised maps.
func NewResponse() *Response {
	res := Response{}
	// Pre-initialise the maps. Synapse will return {} even if there are no rooms under a specific section,
	// so let's do the same thing. Bonus: this means we can't get dreaded 'assignment to entry in nil map' errors.
	res.Rooms.Join = map[string]JoinResponse{}
	res.Rooms.Peek = map[string]JoinResponse{}
	res.Rooms.Invite = map[string]InviteResponse{}
	res.Rooms.Leave = map[string]LeaveResponse{}

	// Also pre-intialise empty slices or else we'll insert 'null' instead of '[]' for the value.
	// TODO: We really shouldn't have to do all this to coerce encoding/json to Do The Right Thing. We should
	//       really be using our own Marshal/Unmarshal implementations otherwise this may prove to be a CPU bottleneck.
	//       This also applies to NewJoinResponse, NewInviteResponse and NewLeaveResponse.
	res.AccountData.Events = []gomatrixserverlib.ClientEvent{}
	res.Presence.Events = []gomatrixserverlib.ClientEvent{}
	res.ToDevice.Events = []gomatrixserverlib.SendToDeviceEvent{}
	res.DeviceListsOTKCount = map[string]int{}

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
	Summary struct {
		Heroes             []string `json:"m.heroes,omitempty"`
		JoinedMemberCount  *int     `json:"m.joined_member_count,omitempty"`
		InvitedMemberCount *int     `json:"m.invited_member_count,omitempty"`
	} `json:"summary"`
	State struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"state"`
	Timeline struct {
		Events    []gomatrixserverlib.ClientEvent `json:"events"`
		Limited   bool                            `json:"limited"`
		PrevBatch *TopologyToken                  `json:"prev_batch,omitempty"`
	} `json:"timeline"`
	Ephemeral struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"ephemeral"`
	AccountData struct {
		Events []gomatrixserverlib.ClientEvent `json:"events"`
	} `json:"account_data"`
	UnreadNotifications struct {
		HighlightCount    int `json:"highlight_count"`
		NotificationCount int `json:"notification_count"`
	} `json:"unread_notifications"`
}

// NewJoinResponse creates an empty response with initialised arrays.
func NewJoinResponse() *JoinResponse {
	res := JoinResponse{}
	res.State.Events = []gomatrixserverlib.ClientEvent{}
	res.Timeline.Events = []gomatrixserverlib.ClientEvent{}
	res.Ephemeral.Events = []gomatrixserverlib.ClientEvent{}
	res.AccountData.Events = []gomatrixserverlib.ClientEvent{}
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
		PrevBatch *TopologyToken                  `json:"prev_batch,omitempty"`
	} `json:"timeline"`
}

// NewLeaveResponse creates an empty response with initialised arrays.
func NewLeaveResponse() *LeaveResponse {
	res := LeaveResponse{}
	res.State.Events = []gomatrixserverlib.ClientEvent{}
	res.Timeline.Events = []gomatrixserverlib.ClientEvent{}
	return &res
}

type SendToDeviceEvent struct {
	gomatrixserverlib.SendToDeviceEvent
	ID       StreamPosition
	UserID   string
	DeviceID string
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

type ReadUpdate struct {
	UserID    string         `json:"user_id"`
	RoomID    string         `json:"room_id"`
	Read      StreamPosition `json:"read,omitempty"`
	FullyRead StreamPosition `json:"fully_read,omitempty"`
}

// StreamEvent is the same as gomatrixserverlib.Event but also has the PDU stream position for this event.
type StreamedEvent struct {
	Event          *gomatrixserverlib.HeaderedEvent `json:"event"`
	StreamPosition StreamPosition                   `json:"stream_position"`
}

// OutputReceiptEvent is an entry in the receipt output kafka log
type OutputReceiptEvent struct {
	UserID    string                      `json:"user_id"`
	RoomID    string                      `json:"room_id"`
	EventID   string                      `json:"event_id"`
	Type      string                      `json:"type"`
	Timestamp gomatrixserverlib.Timestamp `json:"timestamp"`
}

// OutputSendToDeviceEvent is an entry in the send-to-device output kafka log.
// This contains the full event content, along with the user ID and device ID
// to which it is destined.
type OutputSendToDeviceEvent struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	gomatrixserverlib.SendToDeviceEvent
}

type IgnoredUsers struct {
	List map[string]interface{} `json:"ignored_users"`
}
