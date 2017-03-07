package storage

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/roomserver/types"
	"sort"
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
CREATE SEQUENCE IF NOT EXISTS state_block_nid_seq;
CREATE TABLE IF NOT EXISTS state_block (
    -- Local numeric ID for this state data.
    state_block_nid bigint NOT NULL,
    event_type_nid bigint NOT NULL,
    event_state_key_nid bigint NOT NULL,
    event_nid bigint NOT NULL,
    UNIQUE (state_block_nid, event_type_nid, event_state_key_nid)
);
`

const insertStateDataSQL = "" +
	"INSERT INTO state_block (state_block_nid, event_type_nid, event_state_key_nid, event_nid)" +
	" VALUES ($1, $2, $3, $4)"

const selectNextStateBlockNIDSQL = "" +
	"SELECT nextval('state_block_nid_seq')"

// Bulk state lookup by numeric state block ID.
// Sort by the state_block_nid, event_type_nid, event_state_key_nid
// This means that all the entries for a given state_block_nid will appear
// together in the list and those entries will sorted by event_type_nid
// and event_state_key_nid. This property makes it easier to merge two
// state data blocks together.
const bulkSelectStateBlockEntriesSQL = "" +
	"SELECT state_block_nid, event_type_nid, event_state_key_nid, event_nid" +
	" FROM state_block WHERE state_block_nid = ANY($1)" +
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
	" FROM state_block WHERE state_block_nid = ANY($1)" +
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
	sort.Sort(tuples)

	eventTypeNIDArray, eventStateKeyNIDArray := tuples.typesAndStateKeysAsArrays()
	rows, err := s.bulkSelectFilteredStateBlockEntriesStmt.Query(
		stateBlockNIDsAsArray(stateBlockNIDs), eventTypeNIDArray, eventStateKeyNIDArray,
	)
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

		if !tuples.contains(entry.StateKeyTuple) {
			// The select will return the cross product of types and state keys.
			// So we need to check if type of the entry is in the list.
			continue
		}

		if current == nil || types.StateBlockNID(stateBlockNID) != current.StateBlockNID {
			// The state entry row is for a different state data block to the current one.
			// So we start appending to the next entry in the list.
			current = &results[i]
			current.StateBlockNID = types.StateBlockNID(stateBlockNID)
			i++
		}
		current.StateEntries = append(current.StateEntries, entry)
	}
	// Because we have filtered the list it's possible that some of the blocks were completely removed
	// from the result.
	return results[:i], nil
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
	// The event types are already sorted because the tuples were sorted.
	eventTypeNIDs = eventTypeNIDs[:unique(int64Sorter(eventTypeNIDs))]
	// The event state keys need to be sorted.
	sort.Sort(int64Sorter(eventStateKeyNIDs))
	eventStateKeyNIDs = eventStateKeyNIDs[:unique(int64Sorter(eventStateKeyNIDs))]
	return
}

type int64Sorter []int64

func (s int64Sorter) Len() int           { return len(s) }
func (s int64Sorter) Less(i, j int) bool { return s[i] < s[j] }
func (s int64Sorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Remove duplicate items from a sorted list.
// Takes the same interface as sort.Sort
// Returns the length of the data without duplicates
// Uses the last occurance of a duplicate.
// O(n).
func unique(data sort.Interface) int {
	if data.Len() == 0 {
		return 0
	}
	length := data.Len()
	// j is the next index to output an element to.
	j := 0
	for i := 1; i < length; i++ {
		// If the previous element is less than this element then they are
		// not equal. Otherwise they must be equal because the list is sorted.
		// If they are equal then we move onto the next element.
		if data.Less(i-1, i) {
			// "Write" the previous element to the output position by swaping
			// the elements.
			// Note that if the list has no duplicates then i-1 == j so the
			// swap does nothing. (This assumes that data.Swap(a,b) nops if a==b)
			data.Swap(i-1, j)
			// Advance to the next output position in the list.
			j++
		}
	}
	// Output the last element.
	data.Swap(length-1, j)
	return j + 1
}
