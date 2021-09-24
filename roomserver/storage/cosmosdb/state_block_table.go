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
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

// const stateDataSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_state_block (
// 	-- The state snapshot NID that identifies this snapshot.
//     state_block_nid INTEGER PRIMARY KEY AUTOINCREMENT,
// 	-- The hash of the state block, which is used to enforce uniqueness. The hash is
// 	-- generated in Dendrite and passed through to the database, as a btree index over
// 	-- this column is cheap and fits within the maximum index size.
// 	state_block_hash BLOB UNIQUE,
// 	-- The event NIDs contained within the state block, encoded as JSON.
//     event_nids TEXT NOT NULL DEFAULT '[]'
//   );
// `

type stateBlockCosmos struct {
	StateBlockNID  int64   `json:"state_block_nid"`
	StateBlockHash []byte  `json:"state_block_hash"`
	EventNIDs      []int64 `json:"event_nids"`
}

type stateBlockCosmosMaxNID struct {
	Max int64 `json:"maxstateblocknid"`
}

type stateBlockCosmosData struct {
	cosmosdbapi.CosmosDocument
	StateBlock stateBlockCosmos `json:"mx_roomserver_state_block"`
}

// Insert a new state block. If we conflict on the hash column then
// we must perform an update so that the RETURNING statement returns the
// ID of the row that we conflicted with, so that we can then refer to
// the original block.
// const insertStateDataSQL = `
// 	INSERT INTO roomserver_state_block (state_block_hash, event_nids)
// 		VALUES ($1, $2)
// 		ON CONFLICT (state_block_hash) DO UPDATE SET event_nids=$2
// 		RETURNING state_block_nid
// `

// "SELECT state_block_nid, event_nids" +
// " FROM roomserver_state_block WHERE state_block_nid IN ($1) ORDER BY state_block_nid ASC"
const bulkSelectStateBlockEntriesSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_state_block.state_block_nid) " +
	"order by c.mx_roomserver_state_block.state_block_nid "

type stateBlockStatements struct {
	db *Database
	// insertStateDataStmt             *sql.Stmt
	bulkSelectStateBlockEntriesStmt string
	tableName                       string
}

func (s *stateBlockStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *stateBlockStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getStateBlock(s *stateBlockStatements, ctx context.Context, pk string, docId string) (*stateBlockCosmosData, error) {
	response := stateBlockCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response, err
}

func NewCosmosDBStateBlockTable(db *Database) (tables.StateBlock, error) {
	s := &stateBlockStatements{
		db: db,
	}

	// s.insertStateDataStmt = insertStateDataSQL
	s.bulkSelectStateBlockEntriesStmt = bulkSelectStateBlockEntriesSQL
	s.tableName = "state_block"
	return s, nil
}

func (s *stateBlockStatements) BulkInsertStateData(
	ctx context.Context,
	txn *sql.Tx,
	entries types.StateEntries,
) (id types.StateBlockNID, err error) {
	// 	INSERT INTO roomserver_state_block (state_block_hash, event_nids)
	// 		VALUES ($1, $2)
	// 		ON CONFLICT (state_block_hash) DO UPDATE SET event_nids=$2
	// 		RETURNING state_block_nid

	entries = entries[:util.SortAndUnique(entries)]
	nids := types.EventNIDs{} // zero slice to not store 'null' in the DB
	ids := []int64{}
	for _, e := range entries {
		nids = append(nids, e.EventNID)
		ids = append(ids, int64(e.EventNID))
	}
	// js, err := json.Marshal(nids)
	// if err != nil {
	// 	return 0, fmt.Errorf("json.Marshal: %w", err)
	// }
	// err = s.insertStateDataStmt.QueryRowContext(
	// 	ctx, nids.Hash(), js,
	// ).Scan(&id)

	// 	state_block_hash BLOB UNIQUE,
	docId := hex.EncodeToString(nids.Hash())
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	//See if it exists
	existing, err := getStateBlock(s, ctx, s.getPartitionKey(), cosmosDocId)
	if err != nil {
		if err != cosmosdbutil.ErrNoRows {
			return 0, err
		}
	}
	if existing != nil {
		//if exists, just update and dont create a new seq
		existing.SetUpdateTime()
		existing.StateBlock.EventNIDs = ids
		existing.SetUpdateTime()
		_, err = cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, existing.Pk, existing.ETag, existing.Id, existing)
		if err != nil {
			return 0, err
		}
		return types.StateBlockNID(existing.StateBlock.StateBlockNID), nil
	}

	//Doesnt exist,create a new one
	//     state_block_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	seq, err := GetNextStateBlockNID(s, ctx)
	id = types.StateBlockNID(seq)

	data := stateBlockCosmos{
		StateBlockNID:  seq,
		StateBlockHash: nids.Hash(),
		EventNIDs:      ids,
	}

	var dbData = stateBlockCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		StateBlock:     data,
	}

	err = cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)

	return
}

func (s *stateBlockStatements) BulkSelectStateBlockEntries(
	ctx context.Context, stateBlockNIDs types.StateBlockNIDs,
) ([][]types.EventNID, error) {
	// "SELECT state_block_nid, event_nids" +
	// " FROM roomserver_state_block WHERE state_block_nid IN ($1) ORDER BY state_block_nid ASC"

	intfs := make([]interface{}, len(stateBlockNIDs))
	for i := range stateBlockNIDs {
		intfs[i] = int64(stateBlockNIDs[i])
	}
	// selectOrig := strings.Replace(bulkSelectStateBlockEntriesSQL, "($1)", sqlutil.QueryVariadic(len(intfs)), 1)
	// selectStmt, err := s.db.Prepare(selectOrig)
	// if err != nil {
	// 	return nil, err
	// }
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": stateBlockNIDs,
	}

	// rows, err := selectStmt.QueryContext(ctx, intfs...)
	var rows []stateBlockCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectStateBlockEntriesStmt, params, &rows)

	if err != nil {
		return nil, err
	}
	// defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateBlockEntries: rows.close() failed")

	results := make([][]types.EventNID, len(stateBlockNIDs))
	i := 0
	// for ; rows.Next(); i++ {
	for _, item := range rows {
		// var stateBlockNID types.StateBlockNID
		// var result json.RawMessage
		// if err = rows.Scan(&stateBlockNID, &result); err != nil {
		// 	return nil, err
		// }
		r := []types.EventNID{}
		// if err = json.Unmarshal(result, &r); err != nil {
		// 	return nil, fmt.Errorf("json.Unmarshal: %w", err)
		// }
		for _, eventNID := range item.StateBlock.EventNIDs {
			r = append(r, types.EventNID(eventNID))
		}
		results[i] = r
		i++
	}
	// if err = rows.Err(); err != nil {
	// 	return nil, err
	// }
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
