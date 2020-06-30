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
)

var MembershipToEnum = map[string]int{
	gomatrixserverlib.Invite: 1,
	gomatrixserverlib.Join:   2,
	gomatrixserverlib.Leave:  3,
	gomatrixserverlib.Ban:    4,
}
var EnumToMembership = map[int]string{
	1: gomatrixserverlib.Invite,
	2: gomatrixserverlib.Join,
	3: gomatrixserverlib.Leave,
	4: gomatrixserverlib.Ban,
}

type CurrentRoomState interface {
	SelectStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error)
	SelectEventsWithEventIDs(ctx context.Context, txn *sql.Tx, eventIDs []string) ([]gomatrixserverlib.HeaderedEvent, error)
	UpsertRoomState(ctx context.Context, txn *sql.Tx, event gomatrixserverlib.HeaderedEvent, membershipEnum int) error
	DeleteRoomStateByEventID(ctx context.Context, txn *sql.Tx, eventID string) error
	// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
	SelectRoomIDsWithMembership(ctx context.Context, txn *sql.Tx, userID string, membershipEnum int) ([]string, error)
}
