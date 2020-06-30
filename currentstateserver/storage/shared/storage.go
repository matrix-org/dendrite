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

package shared

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/currentstateserver/storage/tables"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database struct {
	DB               *sql.DB
	CurrentRoomState tables.CurrentRoomState
}

func (d *Database) GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error) {
	return d.CurrentRoomState.SelectStateEvent(ctx, roomID, evType, stateKey)
}

func (d *Database) StoreStateEvents(ctx context.Context, addStateEvents []gomatrixserverlib.HeaderedEvent,
	removeStateEventIDs []string) error {
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		// remove first, then add, as we do not ever delete state, but do replace state which is a remove followed by an add.
		for _, eventID := range removeStateEventIDs {
			if err := d.CurrentRoomState.DeleteRoomStateByEventID(ctx, txn, eventID); err != nil {
				return err
			}
		}

		for _, event := range addStateEvents {
			if event.StateKey() == nil {
				// ignore non state events
				continue
			}
			var membershipEnum int
			if event.Type() == "m.room.member" {
				membership, err := event.Membership()
				if err != nil {
					return err
				}
				enum, ok := tables.MembershipToEnum[membership]
				if !ok {
					return fmt.Errorf("unknown membership: %s", membership)
				}
				membershipEnum = enum
			}

			if err := d.CurrentRoomState.UpsertRoomState(ctx, txn, event, membershipEnum); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *Database) GetRoomsByMembership(ctx context.Context, userID, membership string) ([]string, error) {
	enum, ok := tables.MembershipToEnum[membership]
	if !ok {
		return nil, fmt.Errorf("unknown membership: %s", membership)
	}
	return d.CurrentRoomState.SelectRoomIDsWithMembership(ctx, nil, userID, enum)
}
