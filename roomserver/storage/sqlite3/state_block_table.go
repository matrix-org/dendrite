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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

const stateDataSchema = `
  CREATE TABLE IF NOT EXISTS roomserver_state_block (
	-- The state snapshot NID that identifies this snapshot.
    state_block_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	-- The hash of the state block, which is used to enforce uniqueness. The hash is
	-- generated in Dendrite and passed through to the database, as a btree index over 
	-- this column is cheap and fits within the maximum index size.
	state_block_hash BLOB UNIQUE,
	-- The event NIDs contained within the state block, encoded as JSON.
    event_nids TEXT NOT NULL DEFAULT '[]'
  );
`

// Insert a new state block. If we conflict on the hash column then
// we must perform an update so that the RETURNING statement returns the
// ID of the row that we conflicted with, so that we can then refer to
// the original block.
const insertStateDataSQL = `
	INSERT INTO roomserver_state_block (state_block_hash, event_nids)
		VALUES ($1, $2)
		ON CONFLICT (state_block_hash) DO UPDATE SET event_nids=$2
		RETURNING state_block_nid
`

const bulkSelectStateBlockEntriesSQL = "" +
	"SELECT state_block_nid, event_nids" +
	" FROM roomserver_state_block WHERE state_block_nid IN ($1) ORDER BY state_block_nid ASC"

type stateBlockStatements struct {
	db                              *sql.DB
	insertStateDataStmt             *sql.Stmt
	bulkSelectStateBlockEntriesStmt *sql.Stmt
}

func createStateBlockTable(db *sql.DB) error {
	_, err := db.Exec(stateDataSchema)
	return err
}

func prepareStateBlockTable(db *sql.DB) (tables.StateBlock, error) {
	s := &stateBlockStatements{
		db: db,
	}

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
	nids := types.EventNIDs{} // zero slice to not store 'null' in the DB
	for _, e := range entries {
		nids = append(nids, e.EventNID)
	}
	js, err := json.Marshal(nids)
	if err != nil {
		return 0, fmt.Errorf("json.Marshal: %w", err)
	}
	stmt := sqlutil.TxStmt(txn, s.insertStateDataStmt)
	err = stmt.QueryRowContext(
		ctx, nids.Hash(), js,
	).Scan(&id)
	return
}

func (s *stateBlockStatements) BulkSelectStateBlockEntries(
	ctx context.Context, txn *sql.Tx, stateBlockNIDs types.StateBlockNIDs,
) ([][]types.EventNID, error) {
	intfs := make([]interface{}, len(stateBlockNIDs))
	for i := range stateBlockNIDs {
		intfs[i] = int64(stateBlockNIDs[i])
	}
	selectOrig := strings.Replace(bulkSelectStateBlockEntriesSQL, "($1)", sqlutil.QueryVariadic(len(intfs)), 1)
	selectPrep, err := s.db.Prepare(selectOrig)
	if err != nil {
		return nil, err
	}
	defer selectPrep.Close() // nolint:errcheck
	selectStmt := sqlutil.TxStmt(txn, selectPrep)
	rows, err := selectStmt.QueryContext(ctx, intfs...)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateBlockEntries: rows.close() failed")

	results := make([][]types.EventNID, len(stateBlockNIDs))
	i := 0
	for ; rows.Next(); i++ {
		var stateBlockNID types.StateBlockNID
		var result json.RawMessage
		if err = rows.Scan(&stateBlockNID, &result); err != nil {
			return nil, err
		}
		r := []types.EventNID{}
		if err = json.Unmarshal(result, &r); err != nil {
			return nil, fmt.Errorf("json.Unmarshal: %w", err)
		}
		results[i] = r
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(stateBlockNIDs) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", len(results), len(stateBlockNIDs))
	}
	return results, err
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
