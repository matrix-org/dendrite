package input

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"sort"
)

func calculateAndStoreState(
	db RoomEventDatabase, event gomatrixserverlib.Event, roomNID types.RoomNID, stateEventIDs []string,
) (types.StateSnapshotNID, error) {
	if stateEventIDs != nil {
		// 1) We've been told what the state at the event is.
		// Check that those state events are in the database and store the state.
		entries, err := db.StateEntriesForEventIDs(stateEventIDs)
		if err != nil {
			return 0, err
		}

		return db.AddState(roomNID, nil, entries)
	}

	// Load the state at the prev events.
	prevEventRefs := event.PrevEvents()
	prevEventIDs := make([]string, len(prevEventRefs))
	for i := range prevEventRefs {
		prevEventIDs[i] = prevEventRefs[i].EventID
	}

	prevStates, err := db.StateAtEventIDs(prevEventIDs)
	if err != nil {
		return 0, err
	}

	if len(prevStates) == 0 {
		// 2) There weren't any prev_events for this event so the state is
		// empty.
		return db.AddState(roomNID, nil, nil)
	}

	if len(prevStates) == 1 {
		prevState := prevStates[0]
		if prevState.EventStateKeyNID == 0 {
			// 3) None of the previous events were state events and they all
			// have the same state, so this event has exactly the same state
			// as the previous events.
			// This should be the common case.
			return prevState.BeforeStateSnapshotNID, nil
		}
		// The previous event was a state event so we need to store a copy
		// of the previous state updated with that event.
		stateDataNIDLists, err := db.StateDataNIDs([]types.StateSnapshotNID{prevState.BeforeStateSnapshotNID})
		if err != nil {
			return 0, err
		}
		stateDataNIDs := stateDataNIDLists[0].StateDataNIDs
		if len(stateDataNIDs) < maxStateDataNIDs {
			// 4) The number of state data blocks is small enough that we can just
			// add the state event as a block of size one to the end of the blocks.
			return db.AddState(
				roomNID, stateDataNIDs, []types.StateEntry{prevState.StateEntry},
			)
		}
		// If there are too many deltas then we need to calculate the full state
		// So fall through to calculateAndStoreStateMany
	}
	return calculateAndStoreStateMany(db, roomNID, prevStates)
}

// maxStateDataNIDs is the maximum number of state data blocks to use to encode a snapshot of room state.
// Increasing this number means that we can encode more of the state changes as simple deltas which means that
// we need fewer entries in the state data table. However making this number bigger will increase the size of
// the rows in the state table itself and will require more index lookups when retrieving a snapshot.
// TODO: Tune this to get the right balance between size and lookup performance.
const maxStateDataNIDs = 64

func calculateAndStoreStateMany(db RoomEventDatabase, roomNID types.RoomNID, prevStates []types.StateAtEvent) (types.StateSnapshotNID, error) {
	// Conflict resolution.
	// First stage: load the state datablocks for the prev events.
	stateNIDs := make([]types.StateSnapshotNID, len(prevStates))
	for i, state := range prevStates {
		stateNIDs[i] = state.BeforeStateSnapshotNID
	}
	stateDataNIDLists, err := db.StateDataNIDs(uniqueStateSnapshotNIDs(stateNIDs))
	if err != nil {
		return 0, err
	}

	var stateDataNIDs []types.StateDataNID
	for _, list := range stateDataNIDLists {
		stateDataNIDs = append(stateDataNIDs, list.StateDataNIDs...)
	}
	stateEntryLists, err := db.StateEntries(uniqueStateDataNIDs(stateDataNIDs))
	if err != nil {
		return 0, err
	}
	stateDataNIDsMap := stateDataNIDListMap(stateDataNIDLists)
	stateEntriesMap := stateEntryListMap(stateEntryLists)

	var combined []types.StateEntry
	for _, prevState := range prevStates {
		list, ok := stateDataNIDsMap.lookup(prevState.BeforeStateSnapshotNID)
		if !ok {
			// This should only get hit if the database is corrupt.
			// It should be impossible for an event to reference a NID that doesn't exist
			panic(fmt.Errorf("Corrupt DB: Missing state numeric ID %d", prevState.BeforeStateSnapshotNID))
		}

		var fullState []types.StateEntry
		for _, stateDataNID := range list {
			entries, ok := stateEntriesMap.lookup(stateDataNID)
			if !ok {
				// This should only get hit if the database is corrupt.
				// It should be impossible for an event to reference a NID that doesn't exist
				panic(fmt.Errorf("Corrupt DB: Missing state numeric ID %d", prevState.BeforeStateSnapshotNID))
			}
			fullState = append(fullState, entries...)
		}
		if prevState.EventStateKeyNID != 0 {
			fullState = append(fullState, prevState.StateEntry)
		}

		// Stable sort so that the most recent entry for each state key stays
		// remains later in the list than the older entries for the same state key.
		sort.Stable(stateEntryByStateKeySorter(fullState))
		// Unique returns the last entry for each state key.
		fullState = fullState[:unique(stateEntryByStateKeySorter(fullState))]
		// Add the full state for this StateSnapshotNID.
		combined = append(combined, fullState...)
	}

	// Collect all the entries with the same type and key together.
	// We don't care about the order here.
	sort.Sort(stateEntrySorter(combined))
	// Remove duplicate entires.
	combined = combined[:unique(stateEntrySorter(combined))]

	// Find the conflicts
	conflicts := duplicateStateKeys(combined)

	var state []types.StateEntry
	if len(conflicts) > 0 {
		// 5) There are conflicting state events, for each conflict workout
		// what the appropriate state event is.
		resolved, err := resolveConflicts(db, combined, conflicts)
		if err != nil {
			return 0, err
		}
		state = resolved
	} else {
		// 6) There weren't any conflicts
		state = combined
	}

	// TODO: Check if we can encode the new state as a delta against the
	// previous state.
	return db.AddState(roomNID, nil, state)
}

func resolveConflicts(db RoomEventDatabase, combined, conflicted []types.StateEntry) ([]types.StateEntry, error) {
	panic(fmt.Errorf("Not implemented"))
}

func duplicateStateKeys(a []types.StateEntry) []types.StateEntry {
	var result []types.StateEntry
	j := 0
	for i := 1; i < len(a); i++ {
		if a[j].StateKeyTuple != a[i].StateKeyTuple {
			result = append(result, a[j:i]...)
			j = i
		}
	}
	if j != len(a)-1 {
		result = append(result, a[j:]...)
	}
	return result
}

type stateDataNIDListMap []types.StateDataNIDList

func (m stateDataNIDListMap) lookup(stateNID types.StateSnapshotNID) (stateDataNIDs []types.StateDataNID, ok bool) {
	list := []types.StateDataNIDList(m)
	i := sort.Search(len(list), func(i int) bool {
		return list[i].StateSnapshotNID >= stateNID
	})
	if i < len(list) && list[i].StateSnapshotNID == stateNID {
		ok = true
		stateDataNIDs = list[i].StateDataNIDs
	}
	return
}

type stateEntryListMap []types.StateEntryList

func (m stateEntryListMap) lookup(stateDataNID types.StateDataNID) (stateEntries []types.StateEntry, ok bool) {
	list := []types.StateEntryList(m)
	i := sort.Search(len(list), func(i int) bool {
		return list[i].StateDataNID >= stateDataNID
	})
	if i < len(list) && list[i].StateDataNID == stateDataNID {
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

type stateEntrySorter []types.StateEntry

func (s stateEntrySorter) Len() int           { return len(s) }
func (s stateEntrySorter) Less(i, j int) bool { return s[i].LessThan(s[j]) }
func (s stateEntrySorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type stateNIDSorter []types.StateSnapshotNID

func (s stateNIDSorter) Len() int           { return len(s) }
func (s stateNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s stateNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func uniqueStateSnapshotNIDs(nids []types.StateSnapshotNID) []types.StateSnapshotNID {
	sort.Sort(stateNIDSorter(nids))
	return nids[:unique(stateNIDSorter(nids))]
}

type stateDataNIDSorter []types.StateDataNID

func (s stateDataNIDSorter) Len() int           { return len(s) }
func (s stateDataNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s stateDataNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func uniqueStateDataNIDs(nids []types.StateDataNID) []types.StateDataNID {
	sort.Sort(stateDataNIDSorter(nids))
	return nids[:unique(stateDataNIDSorter(nids))]
}

// Remove duplicate items from a sorted list.
// Takes the same interface as sort.Sort
// Returns the length of the date without duplicates
// Uses the last occurance of a duplicate.
// O(n).
func unique(data sort.Interface) int {
	if data.Len() == 0 {
		return 0
	}
	length := data.Len()
	j := 0
	for i := 1; i < length; i++ {
		if data.Less(i-1, i) {
			data.Swap(i-1, j)
			j++
		}
	}
	data.Swap(length-1, j)
	return j + 1
}
