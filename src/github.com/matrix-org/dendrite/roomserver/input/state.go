package input

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"sort"
)

// calculateAndStoreState calculates a snapshot of the state of a room before an event.
// Stores the snapshot of the state in the database.
// Returns a numeric ID for that snapshot.
func calculateAndStoreStateBeforeEvent(
	db RoomEventDatabase, event gomatrixserverlib.Event, roomNID types.RoomNID,
) (types.StateSnapshotNID, error) {
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

	return calculateAndStoreStateAfterEvents(db, roomNID, prevStates)
}

// calculateAndStoreStateAfterEvents finds the room state after the given events.
// Stores the resulting state in the database and returns a numeric ID for that snapshot.
func calculateAndStoreStateAfterEvents(db RoomEventDatabase, roomNID types.RoomNID, prevStates []types.StateAtEvent) (types.StateSnapshotNID, error) {
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
		stateBlockNIDLists, err := db.StateBlockNIDs([]types.StateSnapshotNID{prevState.BeforeStateSnapshotNID})
		if err != nil {
			return 0, err
		}
		stateBlockNIDs := stateBlockNIDLists[0].StateBlockNIDs
		if len(stateBlockNIDs) < maxStateBlockNIDs {
			// 4) The number of state data blocks is small enough that we can just
			// add the state event as a block of size one to the end of the blocks.
			return db.AddState(
				roomNID, stateBlockNIDs, []types.StateEntry{prevState.StateEntry},
			)
		}
		// If there are too many deltas then we need to calculate the full state
		// So fall through to calculateAndStoreStateMany
	}
	return calculateAndStoreStateAfterManyEvents(db, roomNID, prevStates)
}

// maxStateBlockNIDs is the maximum number of state data blocks to use to encode a snapshot of room state.
// Increasing this number means that we can encode more of the state changes as simple deltas which means that
// we need fewer entries in the state data table. However making this number bigger will increase the size of
// the rows in the state table itself and will require more index lookups when retrieving a snapshot.
// TODO: Tune this to get the right balance between size and lookup performance.
const maxStateBlockNIDs = 64

// calculateAndStoreStateAfterManyEvents finds the room state after the given events.
// This handles the slow path of calculateAndStoreStateAfterEvents for when there is more than one event.
// Stores the resulting state and returns a numeric ID for the snapshot.
func calculateAndStoreStateAfterManyEvents(db RoomEventDatabase, roomNID types.RoomNID, prevStates []types.StateAtEvent) (types.StateSnapshotNID, error) {
	// Conflict resolution.
	// First stage: load the state after each of the prev events.
	combined, err := loadCombinedStateAfterEvents(db, prevStates)
	if err != nil {
		return 0, err
	}

	// Collect all the entries with the same type and key together.
	// We don't care about the order here because the conflict resolution
	// algorithm doesn't depend on the order of the prev events.
	sort.Sort(stateEntrySorter(combined))
	// Remove duplicate entires.
	combined = combined[:unique(stateEntrySorter(combined))]

	// Find the conflicts
	conflicts := findDuplicateStateKeys(combined)

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

// loadCombinedStateAfterEvents loads a snapshot of the state after each of the events
// and combines those snapshots together into a single list.
func loadCombinedStateAfterEvents(db RoomEventDatabase, prevStates []types.StateAtEvent) ([]types.StateEntry, error) {
	stateNIDs := make([]types.StateSnapshotNID, len(prevStates))
	for i, state := range prevStates {
		stateNIDs[i] = state.BeforeStateSnapshotNID
	}
	// Fetch the state snapshots for the state before the each prev event from the database.
	// Deduplicate the IDs before passing them to the database.
	// There could be duplicates because the events could be state events where
	// the snapshot of the room state before them was the same.
	stateBlockNIDLists, err := db.StateBlockNIDs(uniqueStateSnapshotNIDs(stateNIDs))
	if err != nil {
		return nil, err
	}

	var stateBlockNIDs []types.StateBlockNID
	for _, list := range stateBlockNIDLists {
		stateBlockNIDs = append(stateBlockNIDs, list.StateBlockNIDs...)
	}
	// Fetch the state entries that will be combined to create the snapshots.
	// Deduplicate the IDs before passing them to the database.
	// There could be duplicates because a block of state entries could be reused by
	// multiple snapshots.
	stateEntryLists, err := db.StateEntries(uniqueStateBlockNIDs(stateBlockNIDs))
	if err != nil {
		return nil, err
	}
	stateBlockNIDsMap := stateBlockNIDListMap(stateBlockNIDLists)
	stateEntriesMap := stateEntryListMap(stateEntryLists)

	// Combine the entries from all the snapshots of state after each prev event into a single list.
	var combined []types.StateEntry
	for _, prevState := range prevStates {
		// Grab the list of state data NIDs for this snapshot.
		stateBlockNIDs, ok := stateBlockNIDsMap.lookup(prevState.BeforeStateSnapshotNID)
		if !ok {
			// This should only get hit if the database is corrupt.
			// It should be impossible for an event to reference a NID that doesn't exist
			panic(fmt.Errorf("Corrupt DB: Missing state numeric ID %d", prevState.BeforeStateSnapshotNID))
		}

		// Combined all the state entries for this snapshot.
		// The order of state data NIDs in the list tells us the order to combine them in.
		var fullState []types.StateEntry
		for _, stateBlockNID := range stateBlockNIDs {
			entries, ok := stateEntriesMap.lookup(stateBlockNID)
			if !ok {
				// This should only get hit if the database is corrupt.
				// It should be impossible for an event to reference a NID that doesn't exist
				panic(fmt.Errorf("Corrupt DB: Missing state numeric ID %d", prevState.BeforeStateSnapshotNID))
			}
			fullState = append(fullState, entries...)
		}
		if prevState.IsStateEvent() {
			// If the prev event was a state event then add an entry for the event itself
			// so that we get the state after the event rather than the state before.
			fullState = append(fullState, prevState.StateEntry)
		}

		// Stable sort so that the most recent entry for each state key stays
		// remains later in the list than the older entries for the same state key.
		sort.Stable(stateEntryByStateKeySorter(fullState))
		// Unique returns the last entry and hence the most recent entry for each state key.
		fullState = fullState[:unique(stateEntryByStateKeySorter(fullState))]
		// Add the full state for this StateSnapshotNID.
		combined = append(combined, fullState...)
	}
	return combined, nil
}

func resolveConflicts(db RoomEventDatabase, combined, conflicted []types.StateEntry) ([]types.StateEntry, error) {
	panic(fmt.Errorf("Not implemented"))
}

// findDuplicateStateKeys finds the state entries where the state key tuple appears more than once in a sorted list.
// Returns a sorted list of those state entries.
func findDuplicateStateKeys(a []types.StateEntry) []types.StateEntry {
	var result []types.StateEntry
	// j is the starting index of a block of entries with the same state key tuple.
	j := 0
	for i := 1; i < len(a); i++ {
		// Check if the state key tuple matches the start of the block
		if a[j].StateKeyTuple != a[i].StateKeyTuple {
			// If the state key tuple is different then we've reached the end of a block of duplicates.
			// Check if the size of the block is bigger than one.
			// If the size is one then there was only a single entry with that state key tuple so we don't add it to the result
			if j+1 != i {
				// Add the block to the result.
				result = append(result, a[j:i]...)
			}
			// Start a new block for the next state key tuple.
			j = i
		}
	}
	// Check if the last block with the same state key tuple had more than one event in it.
	if j+1 != len(a) {
		result = append(result, a[j:]...)
	}
	return result
}

type stateBlockNIDListMap []types.StateBlockNIDList

func (m stateBlockNIDListMap) lookup(stateNID types.StateSnapshotNID) (stateBlockNIDs []types.StateBlockNID, ok bool) {
	list := []types.StateBlockNIDList(m)
	i := sort.Search(len(list), func(i int) bool {
		return list[i].StateSnapshotNID >= stateNID
	})
	if i < len(list) && list[i].StateSnapshotNID == stateNID {
		ok = true
		stateBlockNIDs = list[i].StateBlockNIDs
	}
	return
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

type stateBlockNIDSorter []types.StateBlockNID

func (s stateBlockNIDSorter) Len() int           { return len(s) }
func (s stateBlockNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s stateBlockNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func uniqueStateBlockNIDs(nids []types.StateBlockNID) []types.StateBlockNID {
	sort.Sort(stateBlockNIDSorter(nids))
	return nids[:unique(stateBlockNIDSorter(nids))]
}

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
