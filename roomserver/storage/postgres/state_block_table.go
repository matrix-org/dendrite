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

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

const stateDataSchema = `
-- The state data map.
-- Designed to give enough information to run the state resolution algorithm
-- without hitting the database in the internal case.
-- TODO: Is it worth replacing the unique btree index with a covering index so
-- that postgres could lookup the state using an index-only scan?
-- The type and state_key are included in the index to make it easier to
-- lookup a specific (type, state_key) pair for an event. It also makes it easy
-- to read the state for a given state_block_nid ordered by (type, state_key)
-- which in turn makes it easier to merge state data blocks.
CREATE SEQUENCE IF NOT EXISTS roomserver_state_block_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_state_block (
	-- The state snapshot NID that identifies this snapshot.
	state_block_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_block_nid_seq'),
	-- The hash of the state block, which is used to enforce uniqueness. The hash is
	-- generated in Dendrite and passed through to the database, as a btree index over 
	-- this column is cheap and fits within the maximum index size.
	state_block_hash BYTEA UNIQUE,
	-- The event NIDs contained within the state block.
	event_nids bigint[] NOT NULL
);
`

// Insert a new state block. If we conflict on the hash column then
// we must perform an update so that the RETURNING statement returns the
// ID of the row that we conflicted with, so that we can then refer to
// the original block.
const insertStateDataSQL = "" +
	"INSERT INTO roomserver_state_block (state_block_hash, event_nids)" +
	" VALUES ($1, $2)" +
	" ON CONFLICT (state_block_hash) DO UPDATE SET event_nids=$2" +
	" RETURNING state_block_nid"

const bulkSelectStateBlockEntriesSQL = "" +
	"SELECT state_block_nid, event_nids" +
	" FROM roomserver_state_block WHERE state_block_nid = ANY($1) ORDER BY state_block_nid ASC"

type stateBlockStatements struct {
	insertStateDataStmt             *sql.Stmt
	bulkSelectStateBlockEntriesStmt *sql.Stmt
}

func createStateBlockTable(db *sql.DB) error {
	_, err := db.Exec(stateDataSchema)
	return err
}

func prepareStateBlockTable(db *sql.DB) (tables.StateBlock, error) {
	s := &stateBlockStatements{}

	return s, sqlutil.StatementList{
		{&s.insertStateDataStmt, insertStateDataSQL},
		{&s.bulkSelectStateBlockEntriesStmt, bulkSelectStateBlockEntriesSQL},
	}.Prepare(db)
}

func (s *stateBlockStatements) BulkInsertStateData(
	ctx context.Context, txn *sql.Tx,
	entries types.StateEntries,
) (id types.StateBlockNID, err error) {
	entries = entries[:util.SortAndUnique(entries)]
	var nids types.EventNIDs
	for _, e := range entries {
		nids = append(nids, e.EventNID)
	}
	stmt := sqlutil.TxStmt(txn, s.insertStateDataStmt)
	err = stmt.QueryRowContext(
		ctx, nids.Hash(), eventNIDsAsArray(nids),
	).Scan(&id)
	return
}

func (s *stateBlockStatements) BulkSelectStateBlockEntries(
	ctx context.Context, txn *sql.Tx, stateBlockNIDs types.StateBlockNIDs,
) ([][]types.EventNID, error) {
	stmt := sqlutil.TxStmt(txn, s.bulkSelectStateBlockEntriesStmt)
	rows, err := stmt.QueryContext(ctx, stateBlockNIDsAsArray(stateBlockNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateBlockEntries: rows.close() failed")

	results := make([][]types.EventNID, len(stateBlockNIDs))
	i := 0
	for ; rows.Next(); i++ {
		var stateBlockNID types.StateBlockNID
		var result pq.Int64Array
		if err = rows.Scan(&stateBlockNID, &result); err != nil {
			return nil, err
		}
		r := []types.EventNID{}
		for _, e := range result {
			r = append(r, types.EventNID(e))
		}
		results[i] = r
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(stateBlockNIDs) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", i, len(stateBlockNIDs))
	}
	return results, err
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
