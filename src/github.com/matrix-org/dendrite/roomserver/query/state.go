package query

import (
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"sort"
)

func stringTuplesToNumericTuples(db RoomserverQueryAPIDatabase, stringTuples []api.StateKeyTuple) ([]types.StateKeyTuple, error) {
	eventTypes := make([]string, len(stringTuples))
	stateKeys := make([]string, len(stringTuples))
	for i := range stringTuples {
		eventTypes[i] = stringTuples[i].EventType
		stateKeys[i] = stringTuples[i].EventStateKey
	}
	sort.Strings(eventTypes)
	eventTypes = eventTypes[:unique(sort.StringSlice(eventTypes))]
	eventTypeMap, err := db.EventTypeNIDs(eventTypes)
	if err != nil {
		return nil, err
	}
	sort.Strings(stateKeys)
	stateKeys = stateKeys[:unique(sort.StringSlice(stateKeys))]
	stateKeyMap, err := db.EventStateKeyNIDs(stateKeys)
	if err != nil {
		return nil, err
	}

	var result []types.StateKeyTuple
	for _, stringTuple := range stringTuples {
		var numericTuple types.StateKeyTuple
		var ok1, ok2 bool
		numericTuple.EventTypeNID, ok1 = eventTypeMap[stringTuple.EventType]
		numericTuple.EventStateKeyNID, ok2 = stateKeyMap[stringTuple.EventStateKey]
		if ok1 && ok2 {
			result = append(result, numericTuple)
		}
	}

	return result, nil
}

// loadStateAtSnapshot loads the state of a list of event type and state key pairs in a room at a snapshot.
// This is typically the state before an event or the current state of a room.
// Returns a sorted list of state entries or an error if there was a problem talking to the database.
func loadStateAtSnapshotForTuples(db RoomserverQueryAPIDatabase, stateNID types.StateSnapshotNID, stateKeyTuples []types.StateKeyTuple) ([]types.StateEntry, error) {
	stateBlockNIDLists, err := db.StateBlockNIDs([]types.StateSnapshotNID{stateNID})
	if err != nil {
		return nil, err
	}
	stateBlockNIDList := stateBlockNIDLists[0]

	stateEntryLists, err := db.StateEntriesForTuples(stateBlockNIDList.StateBlockNIDs, stateKeyTuples)
	if err != nil {
		return nil, err
	}
	stateEntriesMap := stateEntryListMap(stateEntryLists)

	// Combined all the state entries for this snapshot.
	// The order of state block NIDs in the list tells us the order to combine them in.
	var fullState []types.StateEntry
	for _, stateBlockNID := range stateBlockNIDList.StateBlockNIDs {
		entries, ok := stateEntriesMap.lookup(stateBlockNID)
		if !ok {
			// If the block is missing from the map it means that none of its entries matched a requested tuple.
			continue
		}
		fullState = append(fullState, entries...)
	}

	// Stable sort so that the most recent entry for each state key stays
	// remains later in the list than the older entries for the same state key.
	sort.Stable(stateEntryByStateKeySorter(fullState))
	// Unique returns the last entry and hence the most recent entry for each state key.
	fullState = fullState[:unique(stateEntryByStateKeySorter(fullState))]
	return fullState, nil
}

type stateEntryListMap []types.StateEntryList

func (m stateEntryListMap) lookup(stateBlockNID types.StateBlockNID) (stateEntries []types.StateEntry, ok bool) {
	list := []types.StateEntryList(m)
	i := sort.Search(len(list), func(i int) bool {
		return list[i].StateBlockNID >= stateBlockNID
	})
	if i < len(list) && list[i].StateBlockNID == stateBlockNID {
		ok = true
		stateEntries = list[i].StateEntries
	}
	return
}

type stateEntryByStateKeySorter []types.StateEntry

func (s stateEntryByStateKeySorter) Len() int { return len(s) }
func (s stateEntryByStateKeySorter) Less(i, j int) bool {
	return s[i].StateKeyTuple.LessThan(s[j].StateKeyTuple)
}
func (s stateEntryByStateKeySorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

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
