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

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

const stateSnapshotSchema = `
-- The state of a room before an event.
-- Stored as a list of state_block entries stored in a separate table.
-- The actual state is constructed by combining all the state_block entries
-- referenced by state_block_nids together. If the same state key tuple appears
-- multiple times then the entry from the later state_block clobbers the earlier
-- entries.
-- This encoding format allows us to implement a delta encoding which is useful
-- because room state tends to accumulate small changes over time. Although if
-- the list of deltas becomes too long it becomes more efficient to encode
-- the full state under single state_block_nid.
CREATE SEQUENCE IF NOT EXISTS roomserver_state_snapshot_nid_seq;
CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
	-- The state snapshot NID that identifies this snapshot.
	state_snapshot_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_snapshot_nid_seq'),
	-- The hash of the state snapshot, which is used to enforce uniqueness. The hash is
	-- generated in Dendrite and passed through to the database, as a btree index over 
	-- this column is cheap and fits within the maximum index size.
	state_snapshot_hash BYTEA UNIQUE,
	-- The room NID that the snapshot belongs to.
	room_nid bigint NOT NULL,
	-- The state blocks contained within this snapshot.
	state_block_nids bigint[] NOT NULL
);
`

// Insert a new state snapshot. If we conflict on the hash column then
// we must perform an update so that the RETURNING statement returns the
// ID of the row that we conflicted with, so that we can then refer to
// the original snapshot.
const insertStateSQL = "" +
	"INSERT INTO roomserver_state_snapshots (state_snapshot_hash, room_nid, state_block_nids)" +
	" VALUES ($1, $2, $3)" +
	" ON CONFLICT (state_snapshot_hash) DO UPDATE SET room_nid=$2" +
	// Performing an update, above, ensures that the RETURNING statement
	// below will always return a valid state snapshot ID
	" RETURNING state_snapshot_nid"

// Bulk state data NID lookup.
// Sorting by state_snapshot_nid means we can use binary search over the result
// to lookup the state data NIDs for a state snapshot NID.
const bulkSelectStateBlockNIDsSQL = "" +
	"SELECT state_snapshot_nid, state_block_nids FROM roomserver_state_snapshots" +
	" WHERE state_snapshot_nid = ANY($1) ORDER BY state_snapshot_nid ASC"

type stateSnapshotStatements struct {
	insertStateStmt              *sql.Stmt
	bulkSelectStateBlockNIDsStmt *sql.Stmt
}

func CreateStateSnapshotTable(db *sql.DB) error {
	_, err := db.Exec(stateSnapshotSchema)
	return err
}

func PrepareStateSnapshotTable(db *sql.DB) (tables.StateSnapshot, error) {
	s := &stateSnapshotStatements{}

	return s, sqlutil.StatementList{
		{&s.insertStateStmt, insertStateSQL},
		{&s.bulkSelectStateBlockNIDsStmt, bulkSelectStateBlockNIDsSQL},
	}.Prepare(db)
}

func (s *stateSnapshotStatements) InsertState(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, nids types.StateBlockNIDs,
) (stateNID types.StateSnapshotNID, err error) {
	nids = nids[:util.SortAndUnique(nids)]
	err = sqlutil.TxStmt(txn, s.insertStateStmt).QueryRowContext(ctx, nids.Hash(), int64(roomNID), stateBlockNIDsAsArray(nids)).Scan(&stateNID)
	if err != nil {
		return 0, err
	}
	return
}

func (s *stateSnapshotStatements) BulkSelectStateBlockNIDs(
	ctx context.Context, txn *sql.Tx, stateNIDs []types.StateSnapshotNID,
) ([]types.StateBlockNIDList, error) {
	nids := make([]int64, len(stateNIDs))
	for i := range stateNIDs {
		nids[i] = int64(stateNIDs[i])
	}
	stmt := sqlutil.TxStmt(txn, s.bulkSelectStateBlockNIDsStmt)
	rows, err := stmt.QueryContext(ctx, pq.Int64Array(nids))
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint: errcheck
	results := make([]types.StateBlockNIDList, len(stateNIDs))
	i := 0
	var stateBlockNIDs pq.Int64Array
	for ; rows.Next(); i++ {
		result := &results[i]
		if err = rows.Scan(&result.StateSnapshotNID, &stateBlockNIDs); err != nil {
			return nil, err
		}
		result.StateBlockNIDs = make([]types.StateBlockNID, len(stateBlockNIDs))
		for k := range stateBlockNIDs {
			result.StateBlockNIDs[k] = types.StateBlockNID(stateBlockNIDs[k])
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if i != len(stateNIDs) {
		return nil, types.MissingStateError(fmt.Sprintf("storage: state NIDs missing from the database (%d != %d)", i, len(stateNIDs)))
	}
	return results, nil
}
