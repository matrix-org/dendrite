// Package state provides functions for reading state from the database.
// The functions for writing state to the database are the input package.
package state

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
	"sort"
)

// A RoomStateDatabase has the storage APIs needed to load state from the database
type RoomStateDatabase interface {
	// Lookup the numeric state data IDs for each numeric state snapshot ID
	// The returned slice is sorted by numeric state snapshot ID.
	StateBlockNIDs(stateNIDs []types.StateSnapshotNID) ([]types.StateBlockNIDList, error)
	// Lookup the state data for each numeric state data ID
	// The returned slice is sorted by numeric state data ID.
	StateEntries(stateBlockNIDs []types.StateBlockNID) ([]types.StateEntryList, error)
}

// LoadStateAtSnapshot loads the full state of a room at a particular snapshot.
// This is typically the state before an event or the current state of a room.
// Returns a sorted list of state entries or an error if there was a problem talking to the database.
func LoadStateAtSnapshot(db RoomStateDatabase, stateNID types.StateSnapshotNID) ([]types.StateEntry, error) {
	stateBlockNIDLists, err := db.StateBlockNIDs([]types.StateSnapshotNID{stateNID})
	if err != nil {
		return nil, err
	}
	stateBlockNIDList := stateBlockNIDLists[0]

	stateEntryLists, err := db.StateEntries(stateBlockNIDList.StateBlockNIDs)
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
			// This should only get hit if the database is corrupt.
			// It should be impossible for an event to reference a NID that doesn't exist
			panic(fmt.Errorf("Corrupt DB: Missing state block numeric ID %d", stateBlockNID))
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

// LoadCombinedStateAfterEvents loads a snapshot of the state after each of the events
// and combines those snapshots together into a single list.
func LoadCombinedStateAfterEvents(db RoomStateDatabase, prevStates []types.StateAtEvent) ([]types.StateEntry, error) {
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
			panic(fmt.Errorf("Corrupt DB: Missing state snapshot numeric ID %d", prevState.BeforeStateSnapshotNID))
		}

		// Combined all the state entries for this snapshot.
		// The order of state block NIDs in the list tells us the order to combine them in.
		var fullState []types.StateEntry
		for _, stateBlockNID := range stateBlockNIDs {
			entries, ok := stateEntriesMap.lookup(stateBlockNID)
			if !ok {
				// This should only get hit if the database is corrupt.
				// It should be impossible for an event to reference a NID that doesn't exist
				panic(fmt.Errorf("Corrupt DB: Missing state block numeric ID %d", stateBlockNID))
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
		fullState = fullState[:util.Unique(stateEntryByStateKeySorter(fullState))]
		// Add the full state for this StateSnapshotNID.
		combined = append(combined, fullState...)
	}
	return combined, nil
}

// DifferenceBetweeenStateSnapshots works out which state entries have been added and removed between two snapshots.
func DifferenceBetweeenStateSnapshots(db RoomStateDatabase, oldStateNID, newStateNID types.StateSnapshotNID) (
	removed, added []types.StateEntry, err error,
) {
	if oldStateNID == newStateNID {
		// If the snapshot NIDs are the same then nothing has changed
		return nil, nil, nil
	}

	var oldEntries []types.StateEntry
	var newEntries []types.StateEntry
	if oldStateNID != 0 {
		oldEntries, err = LoadStateAtSnapshot(db, oldStateNID)
		if err != nil {
			return nil, nil, err
		}
	}
	if newStateNID != 0 {
		newEntries, err = LoadStateAtSnapshot(db, newStateNID)
		if err != nil {
			return nil, nil, err
		}
	}

	var oldI int
	var newI int
	for {
		switch {
		case oldI == len(oldEntries):
			// We've reached the end of the old entries.
			// The rest of the new list must have been newly added.
			added = append(added, newEntries[newI:]...)
			return
		case newI == len(newEntries):
			// We've reached the end of the new entries.
			// The rest of the old list must be have been removed.
			removed = append(removed, oldEntries[oldI:]...)
			return
		case oldEntries[oldI] == newEntries[newI]:
			// The entry is in both lists so skip over it.
			oldI++
			newI++
		case oldEntries[oldI].LessThan(newEntries[newI]):
			// The lists are sorted so the old entry being less than the new entry means that it only appears in the old list.
			removed = append(removed, oldEntries[oldI])
			oldI++
		default:
			// Reaching the default case implies that the new entry is less than the old entry.
			// Since the lists are sorted this means that it only appears in the new list.
			added = append(added, newEntries[newI])
			newI++
		}
	}
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

type stateNIDSorter []types.StateSnapshotNID

func (s stateNIDSorter) Len() int           { return len(s) }
func (s stateNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s stateNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func uniqueStateSnapshotNIDs(nids []types.StateSnapshotNID) []types.StateSnapshotNID {
	return nids[:util.SortAndUnique(stateNIDSorter(nids))]
}

type stateBlockNIDSorter []types.StateBlockNID

func (s stateBlockNIDSorter) Len() int           { return len(s) }
func (s stateBlockNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s stateBlockNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func uniqueStateBlockNIDs(nids []types.StateBlockNID) []types.StateBlockNID {
	return nids[:util.SortAndUnique(stateBlockNIDSorter(nids))]
}
