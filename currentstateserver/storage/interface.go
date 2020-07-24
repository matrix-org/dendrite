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

package storage

import (
	"context"

	"github.com/matrix-org/dendrite/currentstateserver/storage/tables"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	internal.PartitionStorer
	// StoreStateEvents updates the database with new events from the roomserver.
	StoreStateEvents(ctx context.Context, addStateEvents []gomatrixserverlib.HeaderedEvent, removeStateEventIDs []string) error
	// GetStateEvent returns the state event of a given type for a given room with a given state key
	// If no event could be found, returns nil
	// If there was an issue during the retrieval, returns an error
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error)
	// GetRoomsByMembership returns a list of room IDs matching the provided membership and user ID (as state_key).
	GetRoomsByMembership(ctx context.Context, userID, membership string) ([]string, error)
	// GetBulkStateContent returns all state events which match a given room ID and a given state key tuple. Both must be satisfied for a match.
	// If a tuple has the StateKey of '*' and allowWildcards=true then all state events with the EventType should be returned.
	GetBulkStateContent(ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool) ([]tables.StrippedEvent, error)
	// Redact a state event
	RedactEvent(ctx context.Context, redactedEventID string, redactedBecause gomatrixserverlib.HeaderedEvent) error
	// JoinedUsersSetInRooms returns all joined users in the rooms given, along with the count of how many times they appear.
	JoinedUsersSetInRooms(ctx context.Context, roomIDs []string) (map[string]int, error)
}
