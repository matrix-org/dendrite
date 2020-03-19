package shared

import (
	"sort"

	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
)

// findDuplicateStateKeys finds the state entries where the state key tuple appears more than once in a sorted list.
// Returns a sorted list of those state entries.
func FindDuplicateStateKeys(a []types.StateEntry) []types.StateEntry {
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

type StateEntrySorter []types.StateEntry

func (s StateEntrySorter) Len() int           { return len(s) }
func (s StateEntrySorter) Less(i, j int) bool { return s[i].LessThan(s[j]) }
func (s StateEntrySorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type StateBlockNIDListMap []types.StateBlockNIDList

func (m StateBlockNIDListMap) Lookup(stateNID types.StateSnapshotNID) (stateBlockNIDs []types.StateBlockNID, ok bool) {
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

type StateEntryListMap []types.StateEntryList

func (m StateEntryListMap) Lookup(stateBlockNID types.StateBlockNID) (stateEntries []types.StateEntry, ok bool) {
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

type StateEntryByStateKeySorter []types.StateEntry

func (s StateEntryByStateKeySorter) Len() int { return len(s) }
func (s StateEntryByStateKeySorter) Less(i, j int) bool {
	return s[i].StateKeyTuple.LessThan(s[j].StateKeyTuple)
}
func (s StateEntryByStateKeySorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type StateNIDSorter []types.StateSnapshotNID

func (s StateNIDSorter) Len() int           { return len(s) }
func (s StateNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s StateNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func UniqueStateSnapshotNIDs(nids []types.StateSnapshotNID) []types.StateSnapshotNID {
	return nids[:util.SortAndUnique(StateNIDSorter(nids))]
}

type StateBlockNIDSorter []types.StateBlockNID

func (s StateBlockNIDSorter) Len() int           { return len(s) }
func (s StateBlockNIDSorter) Less(i, j int) bool { return s[i] < s[j] }
func (s StateBlockNIDSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func UniqueStateBlockNIDs(nids []types.StateBlockNID) []types.StateBlockNID {
	return nids[:util.SortAndUnique(StateBlockNIDSorter(nids))]
}

// Map from event type, state key tuple to numeric event ID.
// Implemented using binary search on a sorted array.
type StateEntryMap []types.StateEntry

// lookup an entry in the event map.
func (m StateEntryMap) Lookup(stateKey types.StateKeyTuple) (eventNID types.EventNID, ok bool) {
	// Since the list is sorted we can implement this using binary search.
	// This is faster than using a hash map.
	// We don't have to worry about pathological cases because the keys are fixed
	// size and are controlled by us.
	list := []types.StateEntry(m)
	i := sort.Search(len(list), func(i int) bool {
		return !list[i].StateKeyTuple.LessThan(stateKey)
	})
	if i < len(list) && list[i].StateKeyTuple == stateKey {
		ok = true
		eventNID = list[i].EventNID
	}
	return
}

// Map from numeric event ID to event.
// Implemented using binary search on a sorted array.
type EventMap []types.Event

// lookup an entry in the event map.
func (m EventMap) Lookup(eventNID types.EventNID) (event *types.Event, ok bool) {
	// Since the list is sorted we can implement this using binary search.
	// This is faster than using a hash map.
	// We don't have to worry about pathological cases because the keys are fixed
	// size are controlled by us.
	list := []types.Event(m)
	i := sort.Search(len(list), func(i int) bool {
		return list[i].EventNID >= eventNID
	})
	if i < len(list) && list[i].EventNID == eventNID {
		ok = true
		event = &list[i]
	}
	return
}
