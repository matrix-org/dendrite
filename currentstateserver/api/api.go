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

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/gomatrixserverlib"
)

type CurrentStateInternalAPI interface {
	// QueryCurrentState retrieves the requested state events. If state events are not found, they will be missing from
	// the response.
	QueryCurrentState(ctx context.Context, req *QueryCurrentStateRequest, res *QueryCurrentStateResponse) error
	// QueryRoomsForUser retrieves a list of room IDs matching the given query.
	QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error
	// QueryBulkStateContent does a bulk query for state event content in the given rooms.
	QueryBulkStateContent(ctx context.Context, req *QueryBulkStateContentRequest, res *QueryBulkStateContentResponse) error
	// QuerySharedUsers returns a list of users who share at least 1 room in common with the given user.
	QuerySharedUsers(ctx context.Context, req *QuerySharedUsersRequest, res *QuerySharedUsersResponse) error
	// QueryKnownUsers returns a list of users that we know about from our joined rooms.
	QueryKnownUsers(ctx context.Context, req *QueryKnownUsersRequest, res *QueryKnownUsersResponse) error
	// QueryServerBannedFromRoom returns whether a server is banned from a room by server ACLs.
	QueryServerBannedFromRoom(ctx context.Context, req *QueryServerBannedFromRoomRequest, res *QueryServerBannedFromRoomResponse) error
}

type QuerySharedUsersRequest struct {
	UserID         string
	ExcludeRoomIDs []string
	IncludeRoomIDs []string
}

type QuerySharedUsersResponse struct {
	UserIDsToCount map[string]int
}

type QueryRoomsForUserRequest struct {
	UserID string
	// The desired membership of the user. If this is the empty string then no rooms are returned.
	WantMembership string
}

type QueryRoomsForUserResponse struct {
	RoomIDs []string
}

type QueryBulkStateContentRequest struct {
	// Returns state events in these rooms
	RoomIDs []string
	// If true, treats the '*' StateKey as "all state events of this type" rather than a literal value of '*'
	AllowWildcards bool
	// The state events to return. Only a small subset of tuples are allowed in this request as only certain events
	// have their content fields extracted. Specifically, the tuple Type must be one of:
	//   m.room.avatar
	//   m.room.create
	//   m.room.canonical_alias
	//   m.room.guest_access
	//   m.room.history_visibility
	//   m.room.join_rules
	//   m.room.member
	//   m.room.name
	//   m.room.topic
	// Any other tuple type will result in the query failing.
	StateTuples []gomatrixserverlib.StateKeyTuple
}
type QueryBulkStateContentResponse struct {
	// map of room ID -> tuple -> content_value
	Rooms map[string]map[gomatrixserverlib.StateKeyTuple]string
}

type QueryCurrentStateRequest struct {
	RoomID      string
	StateTuples []gomatrixserverlib.StateKeyTuple
}

type QueryCurrentStateResponse struct {
	StateEvents map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent
}

type QueryKnownUsersRequest struct {
	UserID       string `json:"user_id"`
	SearchString string `json:"search_string"`
	Limit        int    `json:"limit"`
}

type QueryKnownUsersResponse struct {
	Users []authtypes.FullyQualifiedProfile `json:"profiles"`
}

type QueryServerBannedFromRoomRequest struct {
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
	RoomID     string                       `json:"room_id"`
}

type QueryServerBannedFromRoomResponse struct {
	Banned bool `json:"banned"`
}

// MarshalJSON stringifies the StateKeyTuple keys so they can be sent over the wire in HTTP API mode.
func (r *QueryCurrentStateResponse) MarshalJSON() ([]byte, error) {
	se := make(map[string]*gomatrixserverlib.HeaderedEvent, len(r.StateEvents))
	for k, v := range r.StateEvents {
		// use 0x1F (unit separator) as the delimiter between type/state key,
		se[fmt.Sprintf("%s\x1F%s", k.EventType, k.StateKey)] = v
	}
	return json.Marshal(se)
}

func (r *QueryCurrentStateResponse) UnmarshalJSON(data []byte) error {
	res := make(map[string]*gomatrixserverlib.HeaderedEvent)
	err := json.Unmarshal(data, &res)
	if err != nil {
		return err
	}
	r.StateEvents = make(map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent, len(res))
	for k, v := range res {
		fields := strings.Split(k, "\x1F")
		r.StateEvents[gomatrixserverlib.StateKeyTuple{
			EventType: fields[0],
			StateKey:  fields[1],
		}] = v
	}
	return nil
}
