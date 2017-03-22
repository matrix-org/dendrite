package input

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/prometheus/client_golang/prometheus"
	"sort"
	"time"
)

var calculateStateDurations = prometheus.NewSummaryVec(
	prometheus.SummaryOpts{
		Namespace: "dendrite",
		Subsystem: "roomserver",
		Name:      "calculate_state_duration_microseconds",
		Help:      "How long it takes to calculate the state after a list of events",
	},
	// Takes two labels:
	//	algorithm:
	//		The algorithm used to calculate the state or the step it failed on if it failed.
	//  outcome:
	//      Whether the state was successfully calculated.
	[]string{"algorithm", "outcome"},
)

type calculateStateTimer struct {
	algorithm string
	startTime time.Time
}

func (c *calculateStateTimer) stop(stateNID types.StateSnapshotNID, err error) (types.StateSnapshotNID, error) {
	var outcome string
	if err == nil {
		outcome = "success"
	} else {
		outcome = "failure"
	}
	endTime := time.Now()
	calculateStateDurations.WithLabelValues(c.algorithm, outcome).Observe(
		float64(endTime.Sub(c.startTime).Nanoseconds()) / 1000.,
	)
	return stateNID, err
}

func init() {
	prometheus.MustRegister(calculateStateDurations)
}

// calculateAndStoreState calculates a snapshot of the state of a room before an event.
// Stores the snapshot of the state in the database.
// Returns a numeric ID for the snapshot of the state before the event.
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

	// The state before this event will be the state after the events that came before it.
	return calculateAndStoreStateAfterEvents(db, roomNID, prevStates)
}

// calculateAndStoreStateAfterEvents finds the room state after the given events.
// Stores the resulting state in the database and returns a numeric ID for that snapshot.
func calculateAndStoreStateAfterEvents(db RoomEventDatabase, roomNID types.RoomNID, prevStates []types.StateAtEvent) (types.StateSnapshotNID, error) {
	timer := calculateStateTimer{startTime: time.Now()}

	if len(prevStates) == 0 {
		// 2) There weren't any prev_events for this event so the state is
		// empty.
		timer.algorithm = "empty_state"
		return timer.stop(db.AddState(roomNID, nil, nil))
	}

	if len(prevStates) == 1 {
		prevState := prevStates[0]
		if prevState.EventStateKeyNID == 0 {
			// 3) None of the previous events were state events and they all
			// have the same state, so this event has exactly the same state
			// as the previous events.
			// This should be the common case.
			timer.algorithm = "no_change"
			return timer.stop(prevState.BeforeStateSnapshotNID, nil)
		}
		// The previous event was a state event so we need to store a copy
		// of the previous state updated with that event.
		stateBlockNIDLists, err := db.StateBlockNIDs([]types.StateSnapshotNID{prevState.BeforeStateSnapshotNID})
		if err != nil {
			timer.algorithm = "_load_state_blocks"
			return timer.stop(0, err)
		}
		stateBlockNIDs := stateBlockNIDLists[0].StateBlockNIDs
		if len(stateBlockNIDs) < maxStateBlockNIDs {
			// 4) The number of state data blocks is small enough that we can just
			// add the state event as a block of size one to the end of the blocks.
			timer.algorithm = "single_delta"
			return timer.stop(db.AddState(
				roomNID, stateBlockNIDs, []types.StateEntry{prevState.StateEntry},
			))
		}
		// If there are too many deltas then we need to calculate the full state
		// So fall through to calculateAndStoreStateAfterManyEvents
	}

	return calculateAndStoreStateAfterManyEvents(db, roomNID, prevStates, timer)
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
func calculateAndStoreStateAfterManyEvents(
	db RoomEventDatabase, roomNID types.RoomNID, prevStates []types.StateAtEvent, timer calculateStateTimer,
) (types.StateSnapshotNID, error) {
	// Conflict resolution.
	// First stage: load the state after each of the prev events.
	combined, err := state.LoadCombinedStateAfterEvents(db, prevStates)
	if err != nil {
		timer.algorithm = "_load_combined_state"
		return timer.stop(0, err)
	}

	// Collect all the entries with the same type and key together.
	// We don't care about the order here because the conflict resolution
	// algorithm doesn't depend on the order of the prev events.
	// Remove duplicate entires.
	combined = combined[:util.SortAndUnique(stateEntrySorter(combined))]

	// Find the conflicts
	conflicts := findDuplicateStateKeys(combined)

	var state []types.StateEntry
	if len(conflicts) > 0 {
		// 5) There are conflicting state events, for each conflict workout
		// what the appropriate state event is.

		// Work out which entries aren't conflicted.
		var notConflicted []types.StateEntry
		for _, entry := range combined {
			if _, ok := stateEntryMap(conflicts).lookup(entry.StateKeyTuple); !ok {
				notConflicted = append(notConflicted, entry)
			}
		}

		resolved, err := resolveConflicts(db, notConflicted, conflicts)
		if err != nil {
			timer.algorithm = "_resolve_conflicts"
			return timer.stop(0, err)
		}
		timer.algorithm = "full_state_with_conflicts"
		state = resolved
	} else {
		timer.algorithm = "full_state_no_conflicts"
		// 6) There weren't any conflicts
		state = combined
	}

	// TODO: Check if we can encode the new state as a delta against the
	// previous state.
	return timer.stop(db.AddState(roomNID, nil, state))
}

// loadStateEvents loads the matrix events for a list of state entries.
// Returns a list of state events in no particular order and a map from string event ID back to state entry.
// The map can be used to recover which numeric state entry a given event is for.
// Returns an error if there was a problem talking to the database.
func loadStateEvents(db RoomEventDatabase, entries []types.StateEntry) ([]gomatrixserverlib.Event, map[string]types.StateEntry, error) {
	eventNIDs := make([]types.EventNID, len(entries))
	for i := range entries {
		eventNIDs[i] = entries[i].EventNID
	}
	events, err := db.Events(eventNIDs)
	if err != nil {
		return nil, nil, err
	}
	eventIDMap := map[string]types.StateEntry{}
	result := make([]gomatrixserverlib.Event, len(entries))
	for i := range entries {
		event, ok := eventMap(events).lookup(entries[i].EventNID)
		if !ok {
			panic(fmt.Errorf("Corrupt DB: Missing event numeric ID %d", entries[i].EventNID))
		}
		result[i] = event.Event
		eventIDMap[event.Event.EventID()] = entries[i]
	}
	return result, eventIDMap, nil
}

// resolveConflicts resolves a list of conflicted state entries. It takes two lists.
// The first is a list of all state entries that are not conflicted.
// The second is a list of all state entries that are conflicted
// A state entry is conflicted when there is more than one numeric event ID for the same state key tuple.
// Returns a list that combines the entries without conflicts with the result of state resolution for the entries with conflicts.
// The returned list is sorted by state key tuple.
// Returns an error if there was a problem talking to the database.
func resolveConflicts(db RoomEventDatabase, notConflicted, conflicted []types.StateEntry) ([]types.StateEntry, error) {

	// Load the conflicted events
	conflictedEvents, eventIDMap, err := loadStateEvents(db, conflicted)
	if err != nil {
		return nil, err
	}

	// Work out which auth events we need to load.
	needed := gomatrixserverlib.StateNeededForAuth(conflictedEvents)

	// Find the numeric IDs for the necessary state keys.
	var neededStateKeys []string
	neededStateKeys = append(neededStateKeys, needed.Member...)
	neededStateKeys = append(neededStateKeys, needed.ThirdPartyInvite...)
	stateKeyNIDMap, err := db.EventStateKeyNIDs(neededStateKeys)
	if err != nil {
		return nil, err
	}

	// Load the necessary auth events.
	tuplesNeeded := stateKeyTuplesNeeded(stateKeyNIDMap, needed)
	var authEntries []types.StateEntry
	for _, tuple := range tuplesNeeded {
		if eventNID, ok := stateEntryMap(notConflicted).lookup(tuple); ok {
			authEntries = append(authEntries, types.StateEntry{tuple, eventNID})
		}
	}
	authEvents, _, err := loadStateEvents(db, authEntries)
	if err != nil {
		return nil, err
	}

	// Resolve the conflicts.
	fmt.Println("Resolving", len(conflicted), "conflicts with", len(authEvents), "authEvents")
	resolvedEvents := gomatrixserverlib.ResolveStateConflicts(conflictedEvents, authEvents)

	// Map from the full events back to numeric state entries.
	for _, resolvedEvent := range resolvedEvents {
		entry, ok := eventIDMap[resolvedEvent.EventID()]
		if !ok {
			panic(fmt.Errorf("Missing state entry for event ID %q", resolvedEvent.EventID()))
		}
		notConflicted = append(notConflicted, entry)
	}

	// Sort the result so it can be searched.
	sort.Sort(stateEntrySorter(notConflicted))
	return notConflicted, nil
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

type stateEntrySorter []types.StateEntry

func (s stateEntrySorter) Len() int           { return len(s) }
func (s stateEntrySorter) Less(i, j int) bool { return s[i].LessThan(s[j]) }
func (s stateEntrySorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
