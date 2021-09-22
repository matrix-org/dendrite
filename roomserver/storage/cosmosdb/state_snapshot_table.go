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

package cosmosdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

// const stateSnapshotSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
//     state_snapshot_nid INTEGER PRIMARY KEY AUTOINCREMENT,
//     room_nid INTEGER NOT NULL,
//     state_block_nids TEXT NOT NULL DEFAULT '[]'
//   );
// `

// CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
// 	-- The state snapshot NID that identifies this snapshot.
//     state_snapshot_nid INTEGER PRIMARY KEY AUTOINCREMENT,
// 	-- The hash of the state snapshot, which is used to enforce uniqueness. The hash is
// 	-- generated in Dendrite and passed through to the database, as a btree index over
// 	-- this column is cheap and fits within the maximum index size.
// 	state_snapshot_hash BLOB UNIQUE,
// 	-- The room NID that the snapshot belongs to.
//     room_nid INTEGER NOT NULL,
// 	-- The state blocks contained within this snapshot, encoded as JSON.
//     state_block_nids TEXT NOT NULL DEFAULT '[]'
//   );

type stateSnapshotCosmos struct {
	StateSnapshotNID  int64   `json:"state_snapshot_nid"`
	StateSnapshotHash []byte  `json:"state_snapshot_hash"`
	RoomNID           int64   `json:"room_nid"`
	StateBlockNIDs    []int64 `json:"state_block_nids"`
}

type stateSnapshotCosmosData struct {
	cosmosdbapi.CosmosDocument
	StateSnapshot stateSnapshotCosmos `json:"mx_roomserver_state_snapshot"`
}

// const insertStateSQL = `
// INSERT INTO roomserver_state_snapshots (state_snapshot_hash, room_nid, state_block_nids)
// VALUES ($1, $2, $3)
// ON CONFLICT (state_snapshot_hash) DO UPDATE SET room_nid=$2
// RETURNING state_snapshot_nid

// Bulk state data NID lookup.
// Sorting by state_snapshot_nid means we can use binary search over the result
// to lookup the state data NIDs for a state snapshot NID.
// 	"SELECT state_snapshot_nid, state_block_nids FROM roomserver_state_snapshots" +
// 	" WHERE state_snapshot_nid IN ($1) ORDER BY state_snapshot_nid ASC"
const bulkSelectStateBlockNIDsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_state_snapshot.state_snapshot_nid) " +
	"order by c.mx_roomserver_state_snapshot.state_snapshot_nid asc"

type stateSnapshotStatements struct {
	db *Database
	// insertStateStmt              *sql.Stmt
	bulkSelectStateBlockNIDsStmt string
	tableName                    string
}

func (s *stateSnapshotStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *stateSnapshotStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func mapFromStateBlockNIDArray(stateBlockNIDs []types.StateBlockNID) []int64 {
	result := []int64{}
	for i := 0; i < len(stateBlockNIDs); i++ {
		result = append(result, int64(stateBlockNIDs[i]))
	}
	return result
}

func mapToStateBlockNIDArray(stateBlockNIDs []int64) []types.StateBlockNID {
	result := []types.StateBlockNID{}
	for i := 0; i < len(stateBlockNIDs); i++ {
		result = append(result, types.StateBlockNID(stateBlockNIDs[i]))
	}
	return result
}

func NewCosmosDBStateSnapshotTable(db *Database) (tables.StateSnapshot, error) {
	s := &stateSnapshotStatements{
		db: db,
	}

	// return s, shared.StatementList{
	// 	{&s.insertStateStmt, insertStateSQL},
	s.bulkSelectStateBlockNIDsStmt = bulkSelectStateBlockNIDsSQL
	// }.Prepare(db)
	s.tableName = "state_snapshots"
	return s, nil
}

func (s *stateSnapshotStatements) InsertState(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID, stateBlockNIDs types.StateBlockNIDs,
) (stateNID types.StateSnapshotNID, err error) {

	// INSERT INTO roomserver_state_snapshots (state_snapshot_hash, room_nid, state_block_nids)
	// VALUES ($1, $2, $3)
	// ON CONFLICT (state_snapshot_hash) DO UPDATE SET room_nid=$2
	// RETURNING state_snapshot_nid
	stateSnapshotNIDSeq, seqErr := GetNextStateSnapshotNID(s, ctx)
	if seqErr != nil {
		return 0, seqErr
	}

	if stateBlockNIDs == nil {
		stateBlockNIDs = []types.StateBlockNID{} // zero slice to not store 'null' in the DB
	}
	stateBlockNIDs = stateBlockNIDs[:util.SortAndUnique(stateBlockNIDs)]
	// stateBlockNIDsJSON, err := json.Marshal(stateBlockNIDs)
	// if err != nil {
	// 	return
	// }

	//     state_snapshot_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	docId := fmt.Sprintf("%d", stateSnapshotNIDSeq)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := stateSnapshotCosmos{
		RoomNID:           int64(roomNID),
		StateSnapshotHash: stateBlockNIDs.Hash(),
		StateBlockNIDs:    mapFromStateBlockNIDArray(stateBlockNIDs),
		StateSnapshotNID:  int64(stateSnapshotNIDSeq),
	}

	var dbData = stateSnapshotCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		StateSnapshot:  data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	if err != nil {
		return 0, err
	}

	stateNID = types.StateSnapshotNID(stateSnapshotNIDSeq)
	return
}

func (s *stateSnapshotStatements) BulkSelectStateBlockNIDs(
	ctx context.Context, stateNIDs []types.StateSnapshotNID,
) ([]types.StateBlockNIDList, error) {

	// "SELECT state_snapshot_nid, state_block_nids FROM roomserver_state_snapshots" +
	// " WHERE state_snapshot_nid IN ($1) ORDER BY state_snapshot_nid ASC"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": stateNIDs,
	}

	var rows []stateSnapshotCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectStateBlockNIDsStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	results := make([]types.StateBlockNIDList, len(stateNIDs))
	i := 0
	for _, item := range rows {
		result := &results[i]
		result.StateSnapshotNID = types.StateSnapshotNID(item.StateSnapshot.StateSnapshotNID)
		result.StateBlockNIDs = mapToStateBlockNIDArray(item.StateSnapshot.StateBlockNIDs)
		i++
	}
	if i != len(stateNIDs) {
		return nil, fmt.Errorf("storage: state NIDs missing from the database (%d != %d)", i, len(stateNIDs))
	}
	return results, nil
}
