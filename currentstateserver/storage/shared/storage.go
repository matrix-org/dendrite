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
	"github.com/matrix-org/dendrite/internal/eventutil"
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

func (d *Database) GetBulkStateContent(ctx context.Context, roomIDs []string, tuples []gomatrixserverlib.StateKeyTuple, allowWildcards bool) ([]tables.StrippedEvent, error) {
	return d.CurrentRoomState.SelectBulkStateContent(ctx, roomIDs, tuples, allowWildcards)
}

func (d *Database) RedactEvent(ctx context.Context, redactedEventID string, redactedBecause gomatrixserverlib.HeaderedEvent) error {
	events, err := d.CurrentRoomState.SelectEventsWithEventIDs(ctx, nil, []string{redactedEventID})
	if err != nil {
		return err
	}
	if len(events) != 1 {
		// this will happen for all non-state events
		return nil
	}
	redactionEvent := redactedBecause.Unwrap()
	eventBeingRedacted := events[0].Unwrap()
	redactedEvent, err := eventutil.RedactEvent(&redactionEvent, &eventBeingRedacted)
	if err != nil {
		return fmt.Errorf("RedactEvent failed: %w", err)
	}
	// replace the state event with a redacted version of itself
	return d.StoreStateEvents(ctx, []gomatrixserverlib.HeaderedEvent{redactedEvent.Headered(redactedBecause.RoomVersion)}, []string{redactedEventID})
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
			contentVal := tables.ExtractContentValue(&event)

			if err := d.CurrentRoomState.UpsertRoomState(ctx, txn, event, contentVal); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *Database) GetRoomsByMembership(ctx context.Context, userID, membership string) ([]string, error) {
	return d.CurrentRoomState.SelectRoomIDsWithMembership(ctx, nil, userID, membership)
}

func (d *Database) JoinedUsersSetInRooms(ctx context.Context, roomIDs []string) (map[string]int, error) {
	return d.CurrentRoomState.SelectJoinedUsersSetForRooms(ctx, roomIDs)
}
