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
	"sort"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

// const stateDataSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_state_block (
//     state_block_nid INTEGER NOT NULL,
//     event_type_nid INTEGER NOT NULL,
//     event_state_key_nid INTEGER NOT NULL,
//     event_nid INTEGER NOT NULL,
//     UNIQUE (state_block_nid, event_type_nid, event_state_key_nid)
//   );
// `

type StateBlockCosmos struct {
	StateBlockNID    int64 `json:"state_block_nid"`
	EventTypeNID     int64 `json:"event_type_nid"`
	EventStateKeyNID int64 `json:"event_state_key_nid"`
	EventNID         int64 `json:"event_nid"`
}

type StateBlockCosmosMaxNID struct {
	Max int64 `json:"maxstateblocknid"`
}

type StateBlockCosmosData struct {
	Id         string           `json:"id"`
	Pk         string           `json:"_pk"`
	Cn         string           `json:"_cn"`
	ETag       string           `json:"_etag"`
	Timestamp  int64            `json:"_ts"`
	StateBlock StateBlockCosmos `json:"mx_roomserver_state_block"`
}

// const insertStateDataSQL = "" +
// 	"INSERT INTO roomserver_state_block (state_block_nid, event_type_nid, event_state_key_nid, event_nid)" +
// 	" VALUES ($1, $2, $3, $4)"

// SELECT IFNULL(MAX(state_block_nid), 0) + 1 FROM roomserver_state_block
const selectNextStateBlockNIDSQL = "" +
	"select sub.maxinner != null ? sub.maxinner + 1 : 1 as maxstateblocknid " +
	"from " +
	"(select MAX(c.mx_roomserver_state_block.state_block_nid) maxinner from c where c._cn = @x1) as sub"

// Bulk state lookup by numeric state block ID.
// Sort by the state_block_nid, event_type_nid, event_state_key_nid
// This means that all the entries for a given state_block_nid will appear
// together in the list and those entries will sorted by event_type_nid
// and event_state_key_nid. This property makes it easier to merge two
// state data blocks together.
// 	"SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
// 	" FROM roomserver_state_block WHERE state_block_nid IN ($1)" +
// 	" ORDER BY state_block_nid, event_type_nid, event_state_key_nid"
const bulkSelectStateBlockEntriesSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_state_block.state_block_nid) " +
	"order by c.mx_roomserver_state_block.state_block_nid " +
	// Cant do multi field order by - The order by query does not have a corresponding composite index that it can be served from
	// ", c.mx_roomserver_state_block.event_type_nid " +
	// ", c.mx_roomserver_state_block.event_state_key_nid " +
	" asc"

// Bulk state lookup by numeric state block ID.
// Filters the rows in each block to the requested types and state keys.
// We would like to restrict to particular type state key pairs but we are
// restricted by the query language to pull the cross product of a list
// of types and a list state_keys. So we have to filter the result in the
// application to restrict it to the list of event types and state keys we
// actually wanted.
// 	"SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
// 	" FROM roomserver_state_block WHERE state_block_nid IN ($1)" +
// 	" AND event_type_nid IN ($2) AND event_state_key_nid IN ($3)" +
// 	" ORDER BY state_block_nid, event_type_nid, event_state_key_nid"
const bulkSelectFilteredStateBlockEntriesSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_state_block.state_block_nid) " +
	"and ARRAY_CONTAINS(@x3, c.mx_roomserver_state_block.event_type_nid) " +
	"and ARRAY_CONTAINS(@x4, c.mx_roomserver_state_block.event_state_key_nid) " +
	"order by c.mx_roomserver_state_block.state_block_nid " +
	// Cant do multi field order by - The order by query does not have a corresponding composite index that it can be served from
	// ", c.mx_roomserver_state_block.event_type_nid " +
	// ", c.mx_roomserver_state_block.event_state_key_nid " +
	"asc"

type stateBlockStatements struct {
	db *Database
	// insertStateDataStmt                     *sql.Stmt
	selectNextStateBlockNIDStmt             string
	bulkSelectStateBlockEntriesStmt         string
	bulkSelectFilteredStateBlockEntriesStmt string
	tableName                               string
}

func queryStateBlock(s *stateBlockStatements, ctx context.Context, qry string, params map[string]interface{}) ([]StateBlockCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []StateBlockCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func NewCosmosDBStateBlockTable(db *Database) (tables.StateBlock, error) {
	s := &stateBlockStatements{
		db: db,
	}

	// return s, shared.StatementList{
	// 	{&s.insertStateDataStmt, insertStateDataSQL},
	s.selectNextStateBlockNIDStmt = selectNextStateBlockNIDSQL
	s.bulkSelectStateBlockEntriesStmt = bulkSelectStateBlockEntriesSQL
	s.bulkSelectFilteredStateBlockEntriesStmt = bulkSelectFilteredStateBlockEntriesSQL
	// }.Prepare(db)
	s.tableName = "state_block"
	return s, nil
}

func inertStateBlockCore(s *stateBlockStatements, ctx context.Context, stateBlockNID types.StateBlockNID, entry types.StateEntry) error {

	// "INSERT INTO roomserver_state_block (state_block_nid, event_type_nid, event_state_key_nid, event_nid)" +
	// " VALUES ($1, $2, $3, $4)"
	data := StateBlockCosmos{
		EventNID:         int64(entry.EventNID),
		EventStateKeyNID: int64(entry.EventStateKeyNID),
		EventTypeNID:     int64(entry.EventTypeNID),
		StateBlockNID:    int64(stateBlockNID),
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	//     UNIQUE (state_block_nid, event_type_nid, event_state_key_nid)
	docId := fmt.Sprintf("%d_%d_%d", data.StateBlockNID, data.EventTypeNID, data.EventStateKeyNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	var dbData = StateBlockCosmosData{
		Id:         cosmosDocId,
		Cn:         dbCollectionName,
		Pk:         pk,
		Timestamp:  time.Now().Unix(),
		StateBlock: data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err

}

func getNextStateBlockNID(s *stateBlockStatements, ctx context.Context) (int64, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var stateBlockNext []StateBlockCosmosMaxNID
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	var optionsQry = cosmosdbapi.GetQueryAllPartitionsDocumentsOptions()
	var query = cosmosdbapi.GetQuery(s.selectNextStateBlockNIDStmt, params)
	var _, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&stateBlockNext,
		optionsQry)

	if err != nil {
		return 0, err
	}

	return stateBlockNext[0].Max, nil
}

func (s *stateBlockStatements) BulkInsertStateData(
	ctx context.Context, txn *sql.Tx,
	entries []types.StateEntry,
) (types.StateBlockNID, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	nextID, errNextID := getNextStateBlockNID(s, ctx)
	if errNextID != nil {
		return 0, errNextID
	}

	stateBlockNID := types.StateBlockNID(nextID)

	for _, entry := range entries {
		err := inertStateBlockCore(s, ctx, stateBlockNID, entry)
		if err != nil {
			return 0, err
		}
	}
	return stateBlockNID, nil
}

func (s *stateBlockStatements) BulkSelectStateBlockEntries(
	ctx context.Context, stateBlockNIDs []types.StateBlockNID,
) ([]types.StateEntryList, error) {

	// "SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
	// " FROM roomserver_state_block WHERE state_block_nid IN ($1)" +
	// " ORDER BY state_block_nid, event_type_nid, event_state_key_nid"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var response []StateBlockCosmosData
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": stateBlockNIDs,
	}

	response, err := queryStateBlock(s, ctx, s.bulkSelectStateBlockEntriesStmt, params)

	if err != nil {
		return nil, err
	}

	results := make([]types.StateEntryList, len(stateBlockNIDs))
	// current is a pointer to the StateEntryList to append the state entries to.
	var current *types.StateEntryList
	i := 0
	for _, item := range response {
		entry := types.StateEntry{}
		entry.EventTypeNID = types.EventTypeNID(item.StateBlock.EventTypeNID)
		entry.EventStateKeyNID = types.EventStateKeyNID(item.StateBlock.EventStateKeyNID)
		entry.EventNID = types.EventNID(item.StateBlock.EventNID)

		if current == nil || types.StateBlockNID(item.StateBlock.StateBlockNID) != current.StateBlockNID {
			// The state entry row is for a different state data block to the current one.
			// So we start appending to the next entry in the list.
			current = &results[i]
			current.StateBlockNID = types.StateBlockNID(item.StateBlock.StateBlockNID)
			i++
		}
		current.StateEntries = append(current.StateEntries, entry)
	}
	if i != len(stateBlockNIDs) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", i, len(stateBlockNIDs))
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
	// sqlStatement := strings.Replace(bulkSelectFilteredStateBlockEntriesSQL, "($1)", sqlutil.QueryVariadic(len(stateBlockNIDs)), 1)
	// sqlStatement = strings.Replace(sqlStatement, "($2)", sqlutil.QueryVariadicOffset(len(eventTypeNIDArray), len(stateBlockNIDs)), 1)
	// sqlStatement = strings.Replace(sqlStatement, "($3)", sqlutil.QueryVariadicOffset(len(eventStateKeyNIDArray), len(stateBlockNIDs)+len(eventTypeNIDArray)), 1)

	// "SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
	// " FROM roomserver_state_block WHERE state_block_nid IN ($1)" +
	// " AND event_type_nid IN ($2) AND event_state_key_nid IN ($3)" +
	// " ORDER BY state_block_nid, event_type_nid, event_state_key_nid"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var response []StateBlockCosmosData
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": stateBlockNIDs,
		"@x3": eventTypeNIDArray,
		"@x4": eventStateKeyNIDArray,
	}

	response, err := queryStateBlock(s, ctx, s.bulkSelectFilteredStateBlockEntriesStmt, params)

	if err != nil {
		return nil, err
	}

	var results []types.StateEntryList
	var current types.StateEntryList
	for _, item := range response {
		var (
			stateBlockNID    int64
			eventTypeNID     int64
			eventStateKeyNID int64
			eventNID         int64
			entry            types.StateEntry
		)
		stateBlockNID = item.StateBlock.StateBlockNID
		eventTypeNID = item.StateBlock.EventTypeNID
		eventStateKeyNID = item.StateBlock.EventStateKeyNID
		eventNID = item.StateBlock.EventNID
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
