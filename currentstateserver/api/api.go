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

	"github.com/matrix-org/gomatrixserverlib"
)

type CurrentStateInternalAPI interface {
	// QueryRoomsForUser retrieves a list of room IDs matching the given query.
	QueryRoomsForUser(ctx context.Context, req *QueryRoomsForUserRequest, res *QueryRoomsForUserResponse) error
	// QueryBulkStateContent does a bulk query for state event content in the given rooms.
	QueryBulkStateContent(ctx context.Context, req *QueryBulkStateContentRequest, res *QueryBulkStateContentResponse) error
	// QuerySharedUsers returns a list of users who share at least 1 room in common with the given user.
	QuerySharedUsers(ctx context.Context, req *QuerySharedUsersRequest, res *QuerySharedUsersResponse) error
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
