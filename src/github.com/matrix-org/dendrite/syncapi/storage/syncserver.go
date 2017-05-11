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

package storage

import (
	"database/sql"
	// Import the postgres database driver.
	_ "github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// SyncServerDatabase represents a sync server database
type SyncServerDatabase struct {
	db         *sql.DB
	partitions common.PartitionOffsetStatements
	events     outputRoomEventsStatements
	roomstate  currentRoomStateStatements
}

// NewSyncServerDatabase creates a new sync server database
func NewSyncServerDatabase(dataSourceName string) (*SyncServerDatabase, error) {
	var db *sql.DB
	var err error
	if db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	partitions := common.PartitionOffsetStatements{}
	if err = partitions.Prepare(db); err != nil {
		return nil, err
	}
	events := outputRoomEventsStatements{}
	if err = events.prepare(db); err != nil {
		return nil, err
	}
	state := currentRoomStateStatements{}
	if err := state.prepare(db); err != nil {
		return nil, err
	}
	return &SyncServerDatabase{db, partitions, events, state}, nil
}

// WriteEvent into the database. It is not safe to call this function from multiple goroutines, as it would create races
// when generating the stream position for this event. Returns the sync stream position for the inserted event.
// Returns an error if there was a problem inserting this event.
func (d *SyncServerDatabase) WriteEvent(ev *gomatrixserverlib.Event, addStateEventIDs, removeStateEventIDs []string) (streamPos types.StreamPosition, returnErr error) {
	returnErr = runTransaction(d.db, func(txn *sql.Tx) error {
		var err error
		pos, err := d.events.InsertEvent(txn, ev, addStateEventIDs, removeStateEventIDs)
		if err != nil {
			return err
		}
		streamPos = types.StreamPosition(pos)

		if len(addStateEventIDs) == 0 && len(removeStateEventIDs) == 0 {
			// Nothing to do, the event may have just been a message event.
			return nil
		}

		// Update the current room state based on the added/removed state event IDs.
		// In the common case there is a single added event ID which is the state event itself, assuming `ev` is a state event.
		// However, conflict resolution may result in there being different events being added, or even some removed.
		if len(removeStateEventIDs) == 0 && len(addStateEventIDs) == 1 && addStateEventIDs[0] == ev.EventID() {
			// common case
			if err = d.roomstate.UpdateRoomState(txn, []gomatrixserverlib.Event{*ev}, nil); err != nil {
				return err
			}
			return nil
		}

		// uncommon case: we need to fetch the full event for each event ID mentioned, then update room state
		added, err := d.events.Events(txn, addStateEventIDs)
		if err != nil {
			return err
		}
		return d.roomstate.UpdateRoomState(txn, added, removeStateEventIDs)
	})
	return
}

// PartitionOffsets implements common.PartitionStorer
func (d *SyncServerDatabase) PartitionOffsets(topic string) ([]common.PartitionOffset, error) {
	return d.partitions.SelectPartitionOffsets(topic)
}

// SetPartitionOffset implements common.PartitionStorer
func (d *SyncServerDatabase) SetPartitionOffset(topic string, partition int32, offset int64) error {
	return d.partitions.UpsertPartitionOffset(topic, partition, offset)
}

// SyncStreamPosition returns the latest position in the sync stream. Returns 0 if there are no events yet.
func (d *SyncServerDatabase) SyncStreamPosition() (types.StreamPosition, error) {
	id, err := d.events.MaxID(nil)
	if err != nil {
		return types.StreamPosition(0), err
	}
	return types.StreamPosition(id), nil
}

// IncrementalSync returns all the data needed in order to create an incremental sync response.
func (d *SyncServerDatabase) IncrementalSync(userID string, fromPos, toPos types.StreamPosition, numRecentEventsPerRoom int) (data map[string]types.RoomData, returnErr error) {
	data = make(map[string]types.RoomData)
	returnErr = runTransaction(d.db, func(txn *sql.Tx) error {
		roomIDs, err := d.roomstate.SelectRoomIDsWithMembership(txn, userID, "join")
		if err != nil {
			return err
		}

		state, err := d.events.StateBetween(txn, fromPos, toPos)
		if err != nil {
			return err
		}

		for _, roomID := range roomIDs {
			recentEvents, err := d.events.RecentEventsInRoom(txn, roomID, fromPos, toPos, numRecentEventsPerRoom)
			if err != nil {
				return err
			}
			state[roomID] = removeDuplicates(state[roomID], recentEvents)
			roomData := types.RoomData{
				State:        state[roomID],
				RecentEvents: recentEvents,
			}
			data[roomID] = roomData
		}
		return nil
	})
	return
}

// CompleteSync returns all the data needed in order to create a complete sync response.
func (d *SyncServerDatabase) CompleteSync(userID string, numRecentEventsPerRoom int) (pos types.StreamPosition, data map[string]types.RoomData, returnErr error) {
	data = make(map[string]types.RoomData)
	// This needs to be all done in a transaction as we need to do multiple SELECTs, and we need to have
	// a consistent view of the database throughout. This includes extracting the sync stream position.
	returnErr = runTransaction(d.db, func(txn *sql.Tx) error {
		// Get the current stream position which we will base the sync response on.
		id, err := d.events.MaxID(txn)
		if err != nil {
			return err
		}
		pos = types.StreamPosition(id)

		// Extract room state and recent events for all rooms the user is joined to.
		roomIDs, err := d.roomstate.SelectRoomIDsWithMembership(txn, userID, "join")
		if err != nil {
			return err
		}
		for _, roomID := range roomIDs {
			stateEvents, err := d.roomstate.CurrentState(txn, roomID)
			if err != nil {
				return err
			}
			// TODO: When filters are added, we may need to call this multiple times to get enough events.
			//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
			recentEvents, err := d.events.RecentEventsInRoom(txn, roomID, types.StreamPosition(0), pos, numRecentEventsPerRoom)
			if err != nil {
				return err
			}

			stateEvents = removeDuplicates(stateEvents, recentEvents)

			data[roomID] = types.RoomData{
				State:        stateEvents,
				RecentEvents: recentEvents,
			}
		}
		return nil
	})
	return
}

// There may be some overlap where events in stateEvents are already in recentEvents, so filter
// them out so we don't include them twice in the /sync response. They should be in recentEvents
// only, so clients get to the correct state once they have rolled forward.
func removeDuplicates(stateEvents, recentEvents []gomatrixserverlib.Event) []gomatrixserverlib.Event {
	for _, recentEv := range recentEvents {
		if recentEv.StateKey() == nil {
			continue // not a state event
		}
		// TODO: This is a linear scan over all the current state events in this room. This will
		//       be slow for big rooms. We should instead sort the state events by event ID  (ORDER BY)
		//       then do a binary search to find matching events, similar to what roomserver does.
		for j := 0; j < len(stateEvents); j++ {
			if stateEvents[j].EventID() == recentEv.EventID() {
				// overwrite the element to remove with the last element then pop the last element.
				// This is orders of magnitude faster than re-slicing, but doesn't preserve ordering
				// (we don't care about the order of stateEvents)
				stateEvents[j] = stateEvents[len(stateEvents)-1]
				stateEvents = stateEvents[:len(stateEvents)-1]
				break // there shouldn't be multiple events with the same event ID
			}
		}
	}
	return stateEvents
}

func runTransaction(db *sql.DB, fn func(txn *sql.Tx) error) (err error) {
	txn, err := db.Begin()
	if err != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			txn.Rollback()
			panic(r)
		} else if err != nil {
			txn.Rollback()
		} else {
			err = txn.Commit()
		}
	}()
	err = fn(txn)
	return
}
