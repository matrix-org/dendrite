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
	"fmt"
	"sort"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

const stateDataSchema = `
-- The state data map.
-- Designed to give enough information to run the state resolution algorithm
-- without hitting the database in the common case.
-- TODO: Is it worth replacing the unique btree index with a covering index so
-- that postgres could lookup the state using an index-only scan?
-- The type and state_key are included in the index to make it easier to
-- lookup a specific (type, state_key) pair for an event. It also makes it easy
-- to read the state for a given state_block_nid ordered by (type, state_key)
-- which in turn makes it easier to merge state data blocks.
CREATE SEQUENCE IF NOT EXISTS roomserver_state_block_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_state_block (
    -- Local numeric ID for this state data.
    state_block_nid bigint NOT NULL,
    event_type_nid bigint NOT NULL,
    event_state_key_nid bigint NOT NULL,
    event_nid bigint NOT NULL,
    UNIQUE (state_block_nid, event_type_nid, event_state_key_nid)
);
`

const insertStateDataSQL = "" +
	"INSERT INTO roomserver_state_block (state_block_nid, event_type_nid, event_state_key_nid, event_nid)" +
	" VALUES ($1, $2, $3, $4)"

const selectNextStateBlockNIDSQL = "" +
	"SELECT nextval('roomserver_state_block_nid_seq')"

// Bulk state lookup by numeric state block ID.
// Sort by the state_block_nid, event_type_nid, event_state_key_nid
// This means that all the entries for a given state_block_nid will appear
// together in the list and those entries will sorted by event_type_nid
// and event_state_key_nid. This property makes it easier to merge two
// state data blocks together.
const bulkSelectStateBlockEntriesSQL = "" +
	"SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
	" FROM roomserver_state_block WHERE state_block_nid = ANY($1)" +
	" ORDER BY state_block_nid, event_type_nid, event_state_key_nid"

// Bulk state lookup by numeric state block ID.
// Filters the rows in each block to the requested types and state keys.
// We would like to restrict to particular type state key pairs but we are
// restricted by the query language to pull the cross product of a list
// of types and a list state_keys. So we have to filter the result in the
// application to restrict it to the list of event types and state keys we
// actually wanted.
const bulkSelectFilteredStateBlockEntriesSQL = "" +
	"SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
	" FROM roomserver_state_block WHERE state_block_nid = ANY($1)" +
	" AND event_type_nid = ANY($2) AND event_state_key_nid = ANY($3)" +
	" ORDER BY state_block_nid, event_type_nid, event_state_key_nid"

type stateBlockStatements struct {
	insertStateDataStmt                     *sql.Stmt
	selectNextStateBlockNIDStmt             *sql.Stmt
	bulkSelectStateBlockEntriesStmt         *sql.Stmt
	bulkSelectFilteredStateBlockEntriesStmt *sql.Stmt
}

func (s *stateBlockStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(stateDataSchema)
	if err != nil {
		return
	}

	return statementList{
		{&s.insertStateDataStmt, insertStateDataSQL},
		{&s.selectNextStateBlockNIDStmt, selectNextStateBlockNIDSQL},
		{&s.bulkSelectStateBlockEntriesStmt, bulkSelectStateBlockEntriesSQL},
		{&s.bulkSelectFilteredStateBlockEntriesStmt, bulkSelectFilteredStateBlockEntriesSQL},
	}.prepare(db)
}

func (s *stateBlockStatements) bulkInsertStateData(stateBlockNID types.StateBlockNID, entries []types.StateEntry) error {
	for _, entry := range entries {
		_, err := s.insertStateDataStmt.Exec(
			int64(stateBlockNID),
			int64(entry.EventTypeNID),
			int64(entry.EventStateKeyNID),
			int64(entry.EventNID),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *stateBlockStatements) selectNextStateBlockNID() (types.StateBlockNID, error) {
	var stateBlockNID int64
	err := s.selectNextStateBlockNIDStmt.QueryRow().Scan(&stateBlockNID)
	return types.StateBlockNID(stateBlockNID), err
}

func (s *stateBlockStatements) bulkSelectStateBlockEntries(stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error) {
	nids := make([]int64, len(stateBlockNIDs))
	for i := range stateBlockNIDs {
		nids[i] = int64(stateBlockNIDs[i])
	}
	rows, err := s.bulkSelectStateBlockEntriesStmt.Query(pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]types.StateEntryList, len(stateBlockNIDs))
	// current is a pointer to the StateEntryList to append the state entries to.
	var current *types.StateEntryList
	i := 0
	for rows.Next() {
		var (
			stateBlockNID    int64
			eventTypeNID     int64
			eventStateKeyNID int64
			eventNID         int64
			entry            types.StateEntry
		)
		if err := rows.Scan(
			&stateBlockNID, &eventTypeNID, &eventStateKeyNID, &eventNID,
		); err != nil {
			return nil, err
		}
		entry.EventTypeNID = types.EventTypeNID(eventTypeNID)
		entry.EventStateKeyNID = types.EventStateKeyNID(eventStateKeyNID)
		entry.EventNID = types.EventNID(eventNID)
		if current == nil || types.StateBlockNID(stateBlockNID) != current.StateBlockNID {
			// The state entry row is for a different state data block to the current one.
			// So we start appending to the next entry in the list.
			current = &results[i]
			current.StateBlockNID = types.StateBlockNID(stateBlockNID)
			i++
		}
		current.StateEntries = append(current.StateEntries, entry)
	}
	if i != len(stateBlockNIDs) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", i, len(stateBlockNIDs))
	}
	return results, nil
}

func (s *stateBlockStatements) bulkSelectFilteredStateBlockEntries(
	stateBlockNIDs []types.StateBlockNID, stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	tuples := stateKeyTupleSorter(stateKeyTuples)
	// Sort the tuples so that we can run binary search against them as we filter the rows returned by the db.
	sort.Sort(tuples)

	eventTypeNIDArray, eventStateKeyNIDArray := tuples.typesAndStateKeysAsArrays()
	rows, err := s.bulkSelectFilteredStateBlockEntriesStmt.Query(
		stateBlockNIDsAsArray(stateBlockNIDs), eventTypeNIDArray, eventStateKeyNIDArray,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []types.StateEntryList
	var current types.StateEntryList
	for rows.Next() {
		var (
			stateBlockNID    int64
			eventTypeNID     int64
			eventStateKeyNID int64
			eventNID         int64
			entry            types.StateEntry
		)
		if err := rows.Scan(
			&stateBlockNID, &eventTypeNID, &eventStateKeyNID, &eventNID,
		); err != nil {
			return nil, err
		}
		entry.EventTypeNID = types.EventTypeNID(eventTypeNID)
		entry.EventStateKeyNID = types.EventStateKeyNID(eventStateKeyNID)
		entry.EventNID = types.EventNID(eventNID)

		// We can use binary search here because we sorted the tuples earlier
		if !tuples.contains(entry.StateKeyTuple) {
			// The select will return the cross product of types and state keys.
			// So we need to check if type of the entry is in the list.
			continue
		}

		if types.StateBlockNID(stateBlockNID) != current.StateBlockNID {
			// The state entry row is for a different state data block to the current one.
			// So we append the current entry to the results and start adding to a new one.
			// The first time through the loop current will be empty.
			if current.StateEntries != nil {
				results = append(results, current)
			}
			current = types.StateEntryList{StateBlockNID: types.StateBlockNID(stateBlockNID)}
		}
		current.StateEntries = append(current.StateEntries, entry)
	}
	// Add the last entry to the list if it is not empty.
	if current.StateEntries != nil {
		results = append(results, current)
	}
	return results, nil
}

func stateBlockNIDsAsArray(stateBlockNIDs []types.StateBlockNID) pq.Int64Array {
	nids := make([]int64, len(stateBlockNIDs))
	for i := range stateBlockNIDs {
		nids[i] = int64(stateBlockNIDs[i])
	}
	return pq.Int64Array(nids)
}

type stateKeyTupleSorter []types.StateKeyTuple

func (s stateKeyTupleSorter) Len() int           { return len(s) }
func (s stateKeyTupleSorter) Less(i, j int) bool { return s[i].LessThan(s[j]) }
func (s stateKeyTupleSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Check whether a tuple is in the list. Assumes that the list is sorted.
func (s stateKeyTupleSorter) contains(value types.StateKeyTuple) bool {
	i := sort.Search(len(s), func(i int) bool { return !s[i].LessThan(value) })
	return i < len(s) && s[i] == value
}

// List the unique eventTypeNIDs and eventStateKeyNIDs.
// Assumes that the list is sorted.
func (s stateKeyTupleSorter) typesAndStateKeysAsArrays() (eventTypeNIDs pq.Int64Array, eventStateKeyNIDs pq.Int64Array) {
	eventTypeNIDs = make(pq.Int64Array, len(s))
	eventStateKeyNIDs = make(pq.Int64Array, len(s))
	for i := range s {
		eventTypeNIDs[i] = int64(s[i].EventTypeNID)
		eventStateKeyNIDs[i] = int64(s[i].EventStateKeyNID)
	}
	eventTypeNIDs = eventTypeNIDs[:util.SortAndUnique(int64Sorter(eventTypeNIDs))]
	eventStateKeyNIDs = eventStateKeyNIDs[:util.SortAndUnique(int64Sorter(eventStateKeyNIDs))]
	return
}

type int64Sorter []int64

func (s int64Sorter) Len() int           { return len(s) }
func (s int64Sorter) Less(i, j int) bool { return s[i] < s[j] }
func (s int64Sorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
