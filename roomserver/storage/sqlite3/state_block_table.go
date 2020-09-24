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
	"sort"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

const stateDataSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_state_block (
    state_block_nid INTEGER NOT NULL,
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
SELECT IFNULL(MAX(state_block_nid), 0) + 1 FROM roomserver_state_block
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

func NewSqliteStateBlockTable(db *sql.DB) (tables.StateBlock, error) {
	s := &stateBlockStatements{
		db: db,
	}
	_, err := db.Exec(stateDataSchema)
	if err != nil {
		return nil, err
	}

	return s, shared.StatementList{
		{&s.insertStateDataStmt, insertStateDataSQL},
		{&s.selectNextStateBlockNIDStmt, selectNextStateBlockNIDSQL},
		{&s.bulkSelectStateBlockEntriesStmt, bulkSelectStateBlockEntriesSQL},
		{&s.bulkSelectFilteredStateBlockEntriesStmt, bulkSelectFilteredStateBlockEntriesSQL},
	}.Prepare(db)
}

func (s *stateBlockStatements) BulkInsertStateData(
	ctx context.Context, txn *sql.Tx,
	entries []types.StateEntry,
) (types.StateBlockNID, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	var stateBlockNID types.StateBlockNID
	err := sqlutil.TxStmt(txn, s.selectNextStateBlockNIDStmt).QueryRowContext(ctx).Scan(&stateBlockNID)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		_, err = sqlutil.TxStmt(txn, s.insertStateDataStmt).ExecContext(
			ctx,
			int64(stateBlockNID),
			int64(entry.EventTypeNID),
			int64(entry.EventStateKeyNID),
			int64(entry.EventNID),
		)
		if err != nil {
			return 0, err
		}
	}
	return stateBlockNID, err
}

func (s *stateBlockStatements) BulkSelectStateBlockEntries(
	ctx context.Context, stateBlockNIDs []types.StateBlockNID,
) ([]types.StateEntryList, error) {
	nids := make([]interface{}, len(stateBlockNIDs))
	for k, v := range stateBlockNIDs {
		nids[k] = v
	}
	selectOrig := strings.Replace(bulkSelectStateBlockEntriesSQL, "($1)", sqlutil.QueryVariadic(len(nids)), 1)
	selectStmt, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	rows, err := selectStmt.QueryContext(ctx, nids...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateBlockEntries: rows.close() failed")

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
	if i != len(nids) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", i, len(nids))
	}
	return results, nil
}

func (s *stateBlockStatements) BulkSelectFilteredStateBlockEntries(
	ctx context.Context,
	stateBlockNIDs []types.StateBlockNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntryList, error) {
	tuples := stateKeyTupleSorter(stateKeyTuples)
	// Sort the tuples so that we can run binary search against them as we filter the rows returned by the db.
	sort.Sort(tuples)

	eventTypeNIDArray, eventStateKeyNIDArray := tuples.typesAndStateKeysAsArrays()
	sqlStatement := strings.Replace(bulkSelectFilteredStateBlockEntriesSQL, "($1)", sqlutil.QueryVariadic(len(stateBlockNIDs)), 1)
	sqlStatement = strings.Replace(sqlStatement, "($2)", sqlutil.QueryVariadicOffset(len(eventTypeNIDArray), len(stateBlockNIDs)), 1)
	sqlStatement = strings.Replace(sqlStatement, "($3)", sqlutil.QueryVariadicOffset(len(eventStateKeyNIDArray), len(stateBlockNIDs)+len(eventTypeNIDArray)), 1)

	var params []interface{}
	for _, val := range stateBlockNIDs {
		params = append(params, int64(val))
	}
	for _, val := range eventTypeNIDArray {
		params = append(params, val)
	}
	for _, val := range eventStateKeyNIDArray {
		params = append(params, val)
	}

	rows, err := s.db.QueryContext(
		ctx,
		sqlStatement,
		params...,
	)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectFilteredStateBlockEntries: rows.close() failed")

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
func (s stateKeyTupleSorter) typesAndStateKeysAsArrays() (eventTypeNIDs []int64, eventStateKeyNIDs []int64) {
	eventTypeNIDs = make([]int64, len(s))
	eventStateKeyNIDs = make([]int64, len(s))
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
