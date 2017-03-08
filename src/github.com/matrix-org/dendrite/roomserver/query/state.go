package query

import (
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
	"sort"
)

func stringTuplesToNumericTuples(db RoomserverQueryAPIDatabase, stringTuples []api.StateKeyTuple) ([]types.StateKeyTuple, error) {
	eventTypes := make([]string, len(stringTuples))
	stateKeys := make([]string, len(stringTuples))
	for i := range stringTuples {
		eventTypes[i] = stringTuples[i].EventType
		stateKeys[i] = stringTuples[i].EventStateKey
	}
	eventTypes = util.UniqueStrings(eventTypes)
	eventTypeMap, err := db.EventTypeNIDs(eventTypes)
	if err != nil {
		return nil, err
	}
	stateKeys = util.UniqueStrings(stateKeys)
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
	fullState = fullState[:util.Unique(stateEntryByStateKeySorter(fullState))]
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
