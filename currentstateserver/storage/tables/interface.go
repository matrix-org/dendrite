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

package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/tidwall/gjson"
)

type CurrentRoomState interface {
	SelectStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error)
	// SelectEventsWithEventIDs returns the events for the given event IDs. If the event(s) are missing, they are not returned
	// and no error is returned.
	SelectEventsWithEventIDs(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]gomatrixserverlib.HeaderedEvent, error)
	// UpsertRoomState stores the given event in the database, along with an extracted piece of content.
	// The piece of content will vary depending on the event type, and table implementations may use this information to optimise
	// lookups e.g membership lookups. The mapped value of `contentVal` is outlined in ExtractContentValue. An empty `contentVal`
	// means there is nothing to store for this field.
	UpsertRoomState(ctx context.Context, txn *sql.Tx, event gomatrixserverlib.HeaderedEvent, contentVal string) error
	DeleteRoomStateByEventID(ctx context.Context, txn *sql.Tx, eventID string) error
	// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
	SelectRoomIDsWithMembership(ctx context.Context, txn *sql.Tx, userID string, membership string) ([]string, error)
	SelectBulkStateContent(ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool) ([]StrippedEvent, error)
	// SelectJoinedUsersSetForRooms returns the set of all users in the rooms who are joined to any of these rooms, along with the
	// counts of how many rooms they are joined.
	SelectJoinedUsersSetForRooms(ctx context.Context, roomIDs []string) (map[string]int, error)
	// SelectKnownUsers searches all users that userID knows about.
	SelectKnownUsers(ctx context.Context, userID, searchString string, limit int) ([]string, error)
}

// StrippedEvent represents a stripped event for returning extracted content values.
type StrippedEvent struct {
	RoomID       string
	EventType    string
	StateKey     string
	ContentValue string
}

// ExtractContentValue from the given state event. For example, given an m.room.name event with:
//    content: { name: "Foo" }
// this returns "Foo".
func ExtractContentValue(ev *gomatrixserverlib.HeaderedEvent) string {
	content := ev.Content()
	key := ""
	switch ev.Type() {
	case gomatrixserverlib.MRoomCreate:
		key = "creator"
	case gomatrixserverlib.MRoomCanonicalAlias:
		key = "alias"
	case gomatrixserverlib.MRoomHistoryVisibility:
		key = "history_visibility"
	case gomatrixserverlib.MRoomJoinRules:
		key = "join_rule"
	case gomatrixserverlib.MRoomMember:
		key = "membership"
	case gomatrixserverlib.MRoomName:
		key = "name"
	case "m.room.avatar":
		key = "url"
	case "m.room.topic":
		key = "topic"
	case "m.room.guest_access":
		key = "guest_access"
	}
	result := gjson.GetBytes(content, key)
	if !result.Exists() {
		return ""
	}
	// this returns the empty string if this is not a string type
	return result.Str
}
