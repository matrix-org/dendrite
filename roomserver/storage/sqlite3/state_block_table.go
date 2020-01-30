// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"context"
	"database/sql"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

const stateDataSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_state_block (
    state_block_nid INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type_nid INTEGER NOT NULL,
    event_state_key_nid INTEGER NOT NULL,
    event_nid INTEGER NOT NULL,
    UNIQUE (state_block_nid, event_type_nid, event_state_key_nid)
  );
`

const insertStateDataSQL = "" +
	"INSERT INTO roomserver_state_block (state_block_nid, event_type_nid, event_state_key_nid, event_nid)" +
	" VALUES ($1, $2, $3, $4)"

const selectNextStateBlockNIDSQL = `
	SELECT COALESCE((
		SELECT seq+1 AS state_block_nid FROM sqlite_sequence
		WHERE name = 'roomserver_state_block'), 1
	) AS state_block_nid
`

// Bulk state lookup by numeric state block ID.
// Sort by the state_block_nid, event_type_nid, event_state_key_nid
// This means that all the entries for a given state_block_nid will appear
// together in the list and those entries will sorted by event_type_nid
// and event_state_key_nid. This property makes it easier to merge two
// state data blocks together.
const bulkSelectStateBlockEntriesSQL = "" +
	"SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
	" FROM roomserver_state_block WHERE state_block_nid IN ($1)" +
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
	" FROM roomserver_state_block WHERE state_block_nid IN ($1)" +
	" AND event_type_nid IN ($2) AND event_state_key_nid IN ($3)" +
	" ORDER BY state_block_nid, event_type_nid, event_state_key_nid"

type stateBlockStatements struct {
	db                                      *sql.DB
	insertStateDataStmt                     *sql.Stmt
	selectNextStateBlockNIDStmt             *sql.Stmt
	bulkSelectStateBlockEntriesStmt         *sql.Stmt
	bulkSelectFilteredStateBlockEntriesStmt *sql.Stmt
}

func (s *stateBlockStatements) prepare(db *sql.DB) (err error) {
	s.db = db
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

func (s *stateBlockStatements) bulkInsertStateData(
	ctx context.Context, txn *sql.Tx,
	stateBlockNID types.StateBlockNID,
	entries []types.StateEntry,
) error {
	for _, entry := range entries {
		_, err := common.TxStmt(txn, s.insertStateDataStmt).ExecContext(
			ctx,
			int64(stateBlockNID),
			int64(entry.EventTypeNID),
			int64(entry.EventStateKeyNID),
			int64(entry.EventNID),
		)
		if err != nil {
			fmt.Println("bulkInsertStateData s.insertStateDataStmt.ExecContext:", err)
			debug.PrintStack()
			return err
		}
	}
	return nil
}

func (s *stateBlockStatements) selectNextStateBlockNID(
	ctx context.Context,
	txn *sql.Tx,
) (types.StateBlockNID, error) {
	var stateBlockNID int64
	selectStmt := common.TxStmt(txn, s.selectNextStateBlockNIDStmt)
	err := selectStmt.QueryRowContext(ctx).Scan(&stateBlockNID)
	return types.StateBlockNID(stateBlockNID), err
}

func (s *stateBlockStatements) bulkSelectStateBlockEntries(
	ctx context.Context, txn *sql.Tx, stateBlockNIDs []types.StateBlockNID,
) ([]types.StateEntryList, error) {
	///////////////
	nids := make([]interface{}, len(stateBlockNIDs))
	for k, v := range stateBlockNIDs {
		nids[k] = v
	}
	selectOrig := strings.Replace(bulkSelectStateBlockEntriesSQL, "($1)", queryVariadic(len(nids)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	///////////////
	/*
		nids := make([]int64, len(stateBlockNIDs))
		for i := range stateBlockNIDs {
			nids[i] = int64(stateBlockNIDs[i])
		}
	*/
	selectStmt := common.TxStmt(txn, selectPrep)
	rows, err := selectStmt.QueryContext(ctx, nids...)
	if err != nil {
		fmt.Println("bulkSelectStateBlockEntries s.bulkSelectStateBlockEntriesStmt.QueryContext:", err)
		return nil, err
	}
	defer rows.Close() // nolint: errcheck

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
			fmt.Println("bulkSelectStateBlockEntries rows.Scan:", err)
			return nil, err
		}
		fmt.Println("state block NID", stateBlockNID, "event type NID", eventTypeNID, "event state key NID", eventStateKeyNID, "event NID", eventNID)
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
	if i != len(nids) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", i, len(nids))
	}
	return results, nil
}

func (s *stateBlockStatements) bulkSelectFilteredStateBlockEntries(
	ctx context.Context, txn *sql.Tx,
	stateBlockNIDs []types.StateBlockNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	tuples := stateKeyTupleSorter(stateKeyTuples)
	// Sort the tuples so that we can run binary search against them as we filter the rows returned by the db.
	sort.Sort(tuples)

	eventTypeNIDArray, eventStateKeyNIDArray := tuples.typesAndStateKeysAsArrays()
	selectStmt := common.TxStmt(txn, s.bulkSelectFilteredStateBlockEntriesStmt)
	rows, err := selectStmt.QueryContext(
		ctx,
		stateBlockNIDsAsArray(stateBlockNIDs),
		eventTypeNIDArray,
		sqliteIn(eventStateKeyNIDArray),
	)
	if err != nil {
		fmt.Println("bulkSelectFilteredStateBlockEntries s.bulkSelectFilteredStateBlockEntriesStmt.QueryContext:", err)
		return nil, err
	}
	defer rows.Close() // nolint: errcheck

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
			fmt.Println("bulkSelectFilteredStateBlockEntries rows.Scan:", err)
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
