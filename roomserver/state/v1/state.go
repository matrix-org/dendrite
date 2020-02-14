// Copyright 2017 Vector Creations Ltd
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

// Package state provides functions for reading state from the database.
// The functions for writing state to the database are the input package.
package v1

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/matrix-org/dendrite/roomserver/state/database"
	"github.com/matrix-org/dendrite/roomserver/state/shared"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type StateResolutionV1 struct {
	db database.RoomStateDatabase
}

func Prepare(db database.RoomStateDatabase) StateResolutionV1 {
	return StateResolutionV1{
		db: db,
	}
}

// LoadStateAtSnapshot loads the full state of a room at a particular snapshot.
// This is typically the state before an event or the current state of a room.
// Returns a sorted list of state entries or an error if there was a problem talking to the database.
func (v StateResolutionV1) LoadStateAtSnapshot(
	ctx context.Context, stateNID types.StateSnapshotNID,
) ([]types.StateEntry, error) {
	stateBlockNIDLists, err := v.db.StateBlockNIDs(ctx, []types.StateSnapshotNID{stateNID})
	if err != nil {
		return nil, err
	}
	// We've asked for exactly one snapshot from the db so we should have exactly one entry in the result.
	stateBlockNIDList := stateBlockNIDLists[0]

	stateEntryLists, err := v.db.StateEntries(ctx, stateBlockNIDList.StateBlockNIDs)
	if err != nil {
		return nil, err
	}
	stateEntriesMap := shared.StateEntryListMap(stateEntryLists)

	// Combine all the state entries for this snapshot.
	// The order of state block NIDs in the list tells us the order to combine them in.
	var fullState []types.StateEntry
	for _, stateBlockNID := range stateBlockNIDList.StateBlockNIDs {
		entries, ok := stateEntriesMap.Lookup(stateBlockNID)
		if !ok {
			// This should only get hit if the database is corrupt.
			// It should be impossible for an event to reference a NID that doesn't exist
			panic(fmt.Errorf("Corrupt DB: Missing state block numeric ID %d", stateBlockNID))
		}
		fullState = append(fullState, entries...)
	}

	// Stable sort so that the most recent entry for each state key stays
	// remains later in the list than the older entries for the same state key.
	sort.Stable(shared.StateEntryByStateKeySorter(fullState))
	// Unique returns the last entry and hence the most recent entry for each state key.
	fullState = fullState[:util.Unique(shared.StateEntryByStateKeySorter(fullState))]
	return fullState, nil
}

// LoadStateAtEvent loads the full state of a room at a particular event.
func (v StateResolutionV1) LoadStateAtEvent(
	ctx context.Context, eventID string,
) ([]types.StateEntry, error) {
	snapshotNID, err := v.db.SnapshotNIDFromEventID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	stateEntries, err := v.LoadStateAtSnapshot(ctx, snapshotNID)
	if err != nil {
		return nil, err
	}

	return stateEntries, nil
}

// LoadCombinedStateAfterEvents loads a snapshot of the state after each of the events
// and combines those snapshots together into a single list.
func (v StateResolutionV1) LoadCombinedStateAfterEvents(
	ctx context.Context, prevStates []types.StateAtEvent,
) ([]types.StateEntry, error) {
	stateNIDs := make([]types.StateSnapshotNID, len(prevStates))
	for i, state := range prevStates {
		stateNIDs[i] = state.BeforeStateSnapshotNID
	}
	// Fetch the state snapshots for the state before the each prev event from the database.
	// Deduplicate the IDs before passing them to the database.
	// There could be duplicates because the events could be state events where
	// the snapshot of the room state before them was the same.
	stateBlockNIDLists, err := v.db.StateBlockNIDs(ctx, shared.UniqueStateSnapshotNIDs(stateNIDs))
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
	stateEntryLists, err := v.db.StateEntries(ctx, shared.UniqueStateBlockNIDs(stateBlockNIDs))
	if err != nil {
		return nil, err
	}
	stateBlockNIDsMap := shared.StateBlockNIDListMap(stateBlockNIDLists)
	stateEntriesMap := shared.StateEntryListMap(stateEntryLists)

	// Combine the entries from all the snapshots of state after each prev event into a single list.
	var combined []types.StateEntry
	for _, prevState := range prevStates {
		// Grab the list of state data NIDs for this snapshot.
		stateBlockNIDs, ok := stateBlockNIDsMap.Lookup(prevState.BeforeStateSnapshotNID)
		if !ok {
			// This should only get hit if the database is corrupt.
			// It should be impossible for an event to reference a NID that doesn't exist
			panic(fmt.Errorf("Corrupt DB: Missing state snapshot numeric ID %d", prevState.BeforeStateSnapshotNID))
		}

		// Combine all the state entries for this snapshot.
		// The order of state block NIDs in the list tells us the order to combine them in.
		var fullState []types.StateEntry
		for _, stateBlockNID := range stateBlockNIDs {
			entries, ok := stateEntriesMap.Lookup(stateBlockNID)
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
		sort.Stable(shared.StateEntryByStateKeySorter(fullState))
		// Unique returns the last entry and hence the most recent entry for each state key.
		fullState = fullState[:util.Unique(shared.StateEntryByStateKeySorter(fullState))]
		// Add the full state for this StateSnapshotNID.
		combined = append(combined, fullState...)
	}
	return combined, nil
}

// DifferenceBetweeenStateSnapshots works out which state entries have been added and removed between two snapshots.
func (v StateResolutionV1) DifferenceBetweeenStateSnapshots(
	ctx context.Context, oldStateNID, newStateNID types.StateSnapshotNID,
) (removed, added []types.StateEntry, err error) {
	if oldStateNID == newStateNID {
		// If the snapshot NIDs are the same then nothing has changed
		return nil, nil, nil
	}

	var oldEntries []types.StateEntry
	var newEntries []types.StateEntry
	if oldStateNID != 0 {
		oldEntries, err = v.LoadStateAtSnapshot(ctx, oldStateNID)
		if err != nil {
			return nil, nil, err
		}
	}
	if newStateNID != 0 {
		newEntries, err = v.LoadStateAtSnapshot(ctx, newStateNID)
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

// LoadStateAtSnapshotForStringTuples loads the state for a list of event type and state key pairs at a snapshot.
// This is used when we only want to load a subset of the room state at a snapshot.
// If there is no entry for a given event type and state key pair then it will be discarded.
// This is typically the state before an event or the current state of a room.
// Returns a sorted list of state entries or an error if there was a problem talking to the database.
func (v StateResolutionV1) LoadStateAtSnapshotForStringTuples(
	ctx context.Context,
	stateNID types.StateSnapshotNID,
	stateKeyTuples []gomatrixserverlib.StateKeyTuple,
) ([]types.StateEntry, error) {
	numericTuples, err := v.stringTuplesToNumericTuples(ctx, stateKeyTuples)
	if err != nil {
		return nil, err
	}
	return v.loadStateAtSnapshotForNumericTuples(ctx, stateNID, numericTuples)
}

// stringTuplesToNumericTuples converts the string state key tuples into numeric IDs
// If there isn't a numeric ID for either the event type or the event state key then the tuple is discarded.
// Returns an error if there was a problem talking to the database.
func (v StateResolutionV1) stringTuplesToNumericTuples(
	ctx context.Context,
	stringTuples []gomatrixserverlib.StateKeyTuple,
) ([]types.StateKeyTuple, error) {
	eventTypes := make([]string, len(stringTuples))
	stateKeys := make([]string, len(stringTuples))
	for i := range stringTuples {
		eventTypes[i] = stringTuples[i].EventType
		stateKeys[i] = stringTuples[i].StateKey
	}
	eventTypes = util.UniqueStrings(eventTypes)
	eventTypeMap, err := v.db.EventTypeNIDs(ctx, eventTypes)
	if err != nil {
		return nil, err
	}
	stateKeys = util.UniqueStrings(stateKeys)
	stateKeyMap, err := v.db.EventStateKeyNIDs(ctx, stateKeys)
	if err != nil {
		return nil, err
	}

	var result []types.StateKeyTuple
	for _, stringTuple := range stringTuples {
		var numericTuple types.StateKeyTuple
		var ok1, ok2 bool
		numericTuple.EventTypeNID, ok1 = eventTypeMap[stringTuple.EventType]
		numericTuple.EventStateKeyNID, ok2 = stateKeyMap[stringTuple.StateKey]
		// Discard the tuple if there wasn't a numeric ID for either the event type or the state key.
		if ok1 && ok2 {
			result = append(result, numericTuple)
		}
	}

	return result, nil
}

// loadStateAtSnapshotForNumericTuples loads the state for a list of event type and state key pairs at a snapshot.
// This is used when we only want to load a subset of the room state at a snapshot.
// If there is no entry for a given event type and state key pair then it will be discarded.
// This is typically the state before an event or the current state of a room.
// Returns a sorted list of state entries or an error if there was a problem talking to the database.
func (v StateResolutionV1) loadStateAtSnapshotForNumericTuples(
	ctx context.Context,
	stateNID types.StateSnapshotNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntry, error) {
	stateBlockNIDLists, err := v.db.StateBlockNIDs(ctx, []types.StateSnapshotNID{stateNID})
	if err != nil {
		return nil, err
	}
	// We've asked for exactly one snapshot from the db so we should have exactly one entry in the result.
	stateBlockNIDList := stateBlockNIDLists[0]

	stateEntryLists, err := v.db.StateEntriesForTuples(
		ctx, stateBlockNIDList.StateBlockNIDs, stateKeyTuples,
	)
	if err != nil {
		return nil, err
	}
	stateEntriesMap := shared.StateEntryListMap(stateEntryLists)

	// Combine all the state entries for this snapshot.
	// The order of state block NIDs in the list tells us the order to combine them in.
	var fullState []types.StateEntry
	for _, stateBlockNID := range stateBlockNIDList.StateBlockNIDs {
		entries, ok := stateEntriesMap.Lookup(stateBlockNID)
		if !ok {
			// If the block is missing from the map it means that none of its entries matched a requested tuple.
			// This can happen if the block doesn't contain an update for one of the requested tuples.
			// If none of the requested tuples are in the block then it can be safely skipped.
			continue
		}
		fullState = append(fullState, entries...)
	}

	// Stable sort so that the most recent entry for each state key stays
	// remains later in the list than the older entries for the same state key.
	sort.Stable(shared.StateEntryByStateKeySorter(fullState))
	// Unique returns the last entry and hence the most recent entry for each state key.
	fullState = fullState[:util.Unique(shared.StateEntryByStateKeySorter(fullState))]
	return fullState, nil
}

// LoadStateAfterEventsForStringTuples loads the state for a list of event type
// and state key pairs after list of events.
// This is used when we only want to load a subset of the room state after a list of events.
// If there is no entry for a given event type and state key pair then it will be discarded.
// This is typically the state before an event.
// Returns a sorted list of state entries or an error if there was a problem talking to the database.
func (v StateResolutionV1) LoadStateAfterEventsForStringTuples(
	ctx context.Context,
	prevStates []types.StateAtEvent,
	stateKeyTuples []gomatrixserverlib.StateKeyTuple,
) ([]types.StateEntry, error) {
	numericTuples, err := v.stringTuplesToNumericTuples(ctx, stateKeyTuples)
	if err != nil {
		return nil, err
	}
	return v.loadStateAfterEventsForNumericTuples(ctx, prevStates, numericTuples)
}

func (v StateResolutionV1) loadStateAfterEventsForNumericTuples(
	ctx context.Context,
	prevStates []types.StateAtEvent,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntry, error) {
	if len(prevStates) == 1 {
		// Fast path for a single event.
		prevState := prevStates[0]
		result, err := v.loadStateAtSnapshotForNumericTuples(
			ctx, prevState.BeforeStateSnapshotNID, stateKeyTuples,
		)
		if err != nil {
			return nil, err
		}
		if prevState.IsStateEvent() {
			// The result is current the state before the requested event.
			// We want the state after the requested event.
			// If the requested event was a state event then we need to
			// update that key in the result.
			// If the requested event wasn't a state event then the state after
			// it is the same as the state before it.
			for i := range result {
				if result[i].StateKeyTuple == prevState.StateKeyTuple {
					result[i] = prevState.StateEntry
				}
			}
		}
		return result, nil
	}

	// Slow path for more that one event.
	// Load the entire state so that we can do conflict resolution if we need to.
	// TODO: The are some optimistations we could do here:
	//    1) We only need to do conflict resolution if there is a conflict in the
	//       requested tuples so we might try loading just those tuples and then
	//       checking for conflicts.
	//    2) When there is a conflict we still only need to load the state
	//       needed to do conflict resolution which would save us having to load
	//       the full state.

	// TODO: Add metrics for this as it could take a long time for big rooms
	// with large conflicts.
	fullState, _, _, err := v.calculateStateAfterManyEvents(ctx, prevStates)
	if err != nil {
		return nil, err
	}

	// Sort the full state so we can use it as a map.
	sort.Sort(shared.StateEntrySorter(fullState))

	// Filter the full state down to the required tuples.
	var result []types.StateEntry
	for _, tuple := range stateKeyTuples {
		eventNID, ok := shared.StateEntryMap(fullState).Lookup(tuple)
		if ok {
			result = append(result, types.StateEntry{
				StateKeyTuple: tuple,
				EventNID:      eventNID,
			})
		}
	}
	sort.Sort(shared.StateEntrySorter(result))
	return result, nil
}

// CalculateAndStoreStateBeforeEvent calculates a snapshot of the state of a room before an event.
// Stores the snapshot of the state in the database.
// Returns a numeric ID for the snapshot of the state before the event.
func (v StateResolutionV1) CalculateAndStoreStateBeforeEvent(
	ctx context.Context,
	event gomatrixserverlib.Event,
	roomNID types.RoomNID,
) (types.StateSnapshotNID, error) {
	// Load the state at the prev events.
	prevEventRefs := event.PrevEvents()
	prevEventIDs := make([]string, len(prevEventRefs))
	for i := range prevEventRefs {
		prevEventIDs[i] = prevEventRefs[i].EventID
	}

	prevStates, err := v.db.StateAtEventIDs(ctx, prevEventIDs)
	if err != nil {
		return 0, err
	}

	// The state before this event will be the state after the events that came before it.
	return v.CalculateAndStoreStateAfterEvents(ctx, roomNID, prevStates)
}

// CalculateAndStoreStateAfterEvents finds the room state after the given events.
// Stores the resulting state in the database and returns a numeric ID for that snapshot.
func (v StateResolutionV1) CalculateAndStoreStateAfterEvents(
	ctx context.Context,
	roomNID types.RoomNID,
	prevStates []types.StateAtEvent,
) (types.StateSnapshotNID, error) {
	metrics := calculateStateMetrics{startTime: time.Now(), prevEventLength: len(prevStates)}

	if len(prevStates) == 0 {
		// 2) There weren't any prev_events for this event so the state is
		// empty.
		metrics.algorithm = "empty_state"
		return metrics.stop(v.db.AddState(ctx, roomNID, nil, nil))
	}

	if len(prevStates) == 1 {
		prevState := prevStates[0]
		if prevState.EventStateKeyNID == 0 {
			// 3) None of the previous events were state events and they all
			// have the same state, so this event has exactly the same state
			// as the previous events.
			// This should be the common case.
			metrics.algorithm = "no_change"
			return metrics.stop(prevState.BeforeStateSnapshotNID, nil)
		}
		// The previous event was a state event so we need to store a copy
		// of the previous state updated with that event.
		stateBlockNIDLists, err := v.db.StateBlockNIDs(
			ctx, []types.StateSnapshotNID{prevState.BeforeStateSnapshotNID},
		)
		if err != nil {
			metrics.algorithm = "_load_state_blocks"
			return metrics.stop(0, err)
		}
		stateBlockNIDs := stateBlockNIDLists[0].StateBlockNIDs
		if len(stateBlockNIDs) < maxStateBlockNIDs {
			// 4) The number of state data blocks is small enough that we can just
			// add the state event as a block of size one to the end of the blocks.
			metrics.algorithm = "single_delta"
			return metrics.stop(v.db.AddState(
				ctx, roomNID, stateBlockNIDs, []types.StateEntry{prevState.StateEntry},
			))
		}
		// If there are too many deltas then we need to calculate the full state
		// So fall through to calculateAndStoreStateAfterManyEvents
	}

	return v.calculateAndStoreStateAfterManyEvents(ctx, roomNID, prevStates, metrics)
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
func (v StateResolutionV1) calculateAndStoreStateAfterManyEvents(
	ctx context.Context,
	roomNID types.RoomNID,
	prevStates []types.StateAtEvent,
	metrics calculateStateMetrics,
) (types.StateSnapshotNID, error) {

	state, algorithm, conflictLength, err :=
		v.calculateStateAfterManyEvents(ctx, prevStates)
	metrics.algorithm = algorithm
	if err != nil {
		return metrics.stop(0, err)
	}

	// TODO: Check if we can encode the new state as a delta against the
	// previous state.
	metrics.conflictLength = conflictLength
	metrics.fullStateLength = len(state)
	return metrics.stop(v.db.AddState(ctx, roomNID, nil, state))
}

func (v StateResolutionV1) calculateStateAfterManyEvents(
	ctx context.Context, prevStates []types.StateAtEvent,
) (state []types.StateEntry, algorithm string, conflictLength int, err error) {
	var combined []types.StateEntry
	// Conflict resolution.
	// First stage: load the state after each of the prev events.
	combined, err = v.LoadCombinedStateAfterEvents(ctx, prevStates)
	if err != nil {
		algorithm = "_load_combined_state"
		return
	}

	// Collect all the entries with the same type and key together.
	// We don't care about the order here because the conflict resolution
	// algorithm doesn't depend on the order of the prev events.
	// Remove duplicate entires.
	combined = combined[:util.SortAndUnique(shared.StateEntrySorter(combined))]

	// Find the conflicts
	conflicts := shared.FindDuplicateStateKeys(combined)

	if len(conflicts) > 0 {
		conflictLength = len(conflicts)

		// 5) There are conflicting state events, for each conflict workout
		// what the appropriate state event is.

		// Work out which entries aren't conflicted.
		var notConflicted []types.StateEntry
		for _, entry := range combined {
			if _, ok := shared.StateEntryMap(conflicts).Lookup(entry.StateKeyTuple); !ok {
				notConflicted = append(notConflicted, entry)
			}
		}

		var resolved []types.StateEntry
		resolved, err = v.resolveConflicts(ctx, notConflicted, conflicts)
		if err != nil {
			algorithm = "_resolve_conflicts"
			return
		}
		algorithm = "full_state_with_conflicts"
		state = resolved
	} else {
		algorithm = "full_state_no_conflicts"
		// 6) There weren't any conflicts
		state = combined
	}
	return
}

// resolveConflicts resolves a list of conflicted state entries. It takes two lists.
// The first is a list of all state entries that are not conflicted.
// The second is a list of all state entries that are conflicted
// A state entry is conflicted when there is more than one numeric event ID for the same state key tuple.
// Returns a list that combines the entries without conflicts with the result of state resolution for the entries with conflicts.
// The returned list is sorted by state key tuple.
// Returns an error if there was a problem talking to the database.
func (v StateResolutionV1) resolveConflicts(
	ctx context.Context,
	notConflicted, conflicted []types.StateEntry,
) ([]types.StateEntry, error) {

	// Load the conflicted events
	conflictedEvents, eventIDMap, err := v.loadStateEvents(ctx, conflicted)
	if err != nil {
		return nil, err
	}

	// Work out which auth events we need to load.
	needed := gomatrixserverlib.StateNeededForAuth(conflictedEvents)

	// Find the numeric IDs for the necessary state keys.
	var neededStateKeys []string
	neededStateKeys = append(neededStateKeys, needed.Member...)
	neededStateKeys = append(neededStateKeys, needed.ThirdPartyInvite...)
	stateKeyNIDMap, err := v.db.EventStateKeyNIDs(ctx, neededStateKeys)
	if err != nil {
		return nil, err
	}

	// Load the necessary auth events.
	tuplesNeeded := v.stateKeyTuplesNeeded(stateKeyNIDMap, needed)
	var authEntries []types.StateEntry
	for _, tuple := range tuplesNeeded {
		if eventNID, ok := shared.StateEntryMap(notConflicted).Lookup(tuple); ok {
			authEntries = append(authEntries, types.StateEntry{
				StateKeyTuple: tuple,
				EventNID:      eventNID,
			})
		}
	}
	authEvents, _, err := v.loadStateEvents(ctx, authEntries)
	if err != nil {
		return nil, err
	}

	// Resolve the conflicts.
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
	sort.Sort(shared.StateEntrySorter(notConflicted))
	return notConflicted, nil
}

// stateKeyTuplesNeeded works out which numeric state key tuples we need to authenticate some events.
func (v StateResolutionV1) stateKeyTuplesNeeded(stateKeyNIDMap map[string]types.EventStateKeyNID, stateNeeded gomatrixserverlib.StateNeeded) []types.StateKeyTuple {
	var keyTuples []types.StateKeyTuple
	if stateNeeded.Create {
		keyTuples = append(keyTuples, types.StateKeyTuple{
			EventTypeNID:     types.MRoomCreateNID,
			EventStateKeyNID: types.EmptyStateKeyNID,
		})
	}
	if stateNeeded.PowerLevels {
		keyTuples = append(keyTuples, types.StateKeyTuple{
			EventTypeNID:     types.MRoomPowerLevelsNID,
			EventStateKeyNID: types.EmptyStateKeyNID,
		})
	}
	if stateNeeded.JoinRules {
		keyTuples = append(keyTuples, types.StateKeyTuple{
			EventTypeNID:     types.MRoomJoinRulesNID,
			EventStateKeyNID: types.EmptyStateKeyNID,
		})
	}
	for _, member := range stateNeeded.Member {
		stateKeyNID, ok := stateKeyNIDMap[member]
		if ok {
			keyTuples = append(keyTuples, types.StateKeyTuple{
				EventTypeNID:     types.MRoomMemberNID,
				EventStateKeyNID: stateKeyNID,
			})
		}
	}
	for _, token := range stateNeeded.ThirdPartyInvite {
		stateKeyNID, ok := stateKeyNIDMap[token]
		if ok {
			keyTuples = append(keyTuples, types.StateKeyTuple{
				EventTypeNID:     types.MRoomThirdPartyInviteNID,
				EventStateKeyNID: stateKeyNID,
			})
		}
	}
	return keyTuples
}

// loadStateEvents loads the matrix events for a list of state entries.
// Returns a list of state events in no particular order and a map from string event ID back to state entry.
// The map can be used to recover which numeric state entry a given event is for.
// Returns an error if there was a problem talking to the database.
func (v StateResolutionV1) loadStateEvents(
	ctx context.Context, entries []types.StateEntry,
) ([]gomatrixserverlib.Event, map[string]types.StateEntry, error) {
	eventNIDs := make([]types.EventNID, len(entries))
	for i := range entries {
		eventNIDs[i] = entries[i].EventNID
	}
	events, err := v.db.Events(ctx, eventNIDs)
	if err != nil {
		return nil, nil, err
	}
	eventIDMap := map[string]types.StateEntry{}
	result := make([]gomatrixserverlib.Event, len(entries))
	for i := range entries {
		event, ok := shared.EventMap(events).Lookup(entries[i].EventNID)
		if !ok {
			panic(fmt.Errorf("Corrupt DB: Missing event numeric ID %d", entries[i].EventNID))
		}
		result[i] = event.Event
		eventIDMap[event.Event.EventID()] = entries[i]
	}
	return result, eventIDMap, nil
}
