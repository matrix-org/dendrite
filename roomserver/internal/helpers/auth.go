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

package helpers

import (
	"context"
	"fmt"
	"sort"

	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// CheckForSoftFail returns true if the event should be soft-failed
// and false otherwise. The return error value should be checked before
// the soft-fail bool.
func CheckForSoftFail(
	ctx context.Context,
	db storage.Database,
	event *gomatrixserverlib.HeaderedEvent,
	stateEventIDs []string,
) (bool, error) {
	rewritesState := len(stateEventIDs) > 1

	var authStateEntries []types.StateEntry
	var err error
	if rewritesState {
		authStateEntries, err = db.StateEntriesForEventIDs(ctx, stateEventIDs)
		if err != nil {
			return true, fmt.Errorf("StateEntriesForEventIDs failed: %w", err)
		}
	} else {
		// Work out if the room exists.
		var roomInfo *types.RoomInfo
		roomInfo, err = db.RoomInfo(ctx, event.RoomID())
		if err != nil {
			return false, fmt.Errorf("db.RoomNID: %w", err)
		}
		if roomInfo == nil || roomInfo.IsStub {
			return false, nil
		}

		// Then get the state entries for the current state snapshot.
		// We'll use this to check if the event is allowed right now.
		roomState := state.NewStateResolution(db, roomInfo)
		authStateEntries, err = roomState.LoadStateAtSnapshot(ctx, roomInfo.StateSnapshotNID)
		if err != nil {
			return true, fmt.Errorf("roomState.LoadStateAtSnapshot: %w", err)
		}
	}

	// As a special case, it's possible that the room will have no
	// state because we haven't received a m.room.create event yet.
	// If we're now processing the first create event then never
	// soft-fail it.
	if len(authStateEntries) == 0 && event.Type() == gomatrixserverlib.MRoomCreate {
		return false, nil
	}

	// Work out which of the state events we actually need.
	stateNeeded := gomatrixserverlib.StateNeededForAuth([]*gomatrixserverlib.Event{event.Unwrap()})

	// Load the actual auth events from the database.
	authEvents, err := loadAuthEvents(ctx, db, stateNeeded, authStateEntries)
	if err != nil {
		return true, fmt.Errorf("loadAuthEvents: %w", err)
	}

	// Check if the event is allowed.
	if err = gomatrixserverlib.Allowed(event.Event, &authEvents); err != nil {
		// return true, nil
		return true, err
	}
	return false, nil
}

// CheckAuthEvents checks that the event passes authentication checks
// Returns the numeric IDs for the auth events.
func CheckAuthEvents(
	ctx context.Context,
	db storage.Database,
	event *gomatrixserverlib.HeaderedEvent,
	authEventIDs []string,
) ([]types.EventNID, error) {
	// Grab the numeric IDs for the supplied auth state events from the database.
	authStateEntries, err := db.StateEntriesForEventIDs(ctx, authEventIDs)
	if err != nil {
		return nil, fmt.Errorf("db.StateEntriesForEventIDs: %w", err)
	}
	authStateEntries = types.DeduplicateStateEntries(authStateEntries)

	// Work out which of the state events we actually need.
	stateNeeded := gomatrixserverlib.StateNeededForAuth([]*gomatrixserverlib.Event{event.Unwrap()})

	// Load the actual auth events from the database.
	authEvents, err := loadAuthEvents(ctx, db, stateNeeded, authStateEntries)
	if err != nil {
		return nil, fmt.Errorf("loadAuthEvents: %w", err)
	}

	// Check if the event is allowed.
	if err = gomatrixserverlib.Allowed(event.Event, &authEvents); err != nil {
		return nil, err
	}

	// Return the numeric IDs for the auth events.
	result := make([]types.EventNID, len(authStateEntries))
	for i := range authStateEntries {
		result[i] = authStateEntries[i].EventNID
	}
	return result, nil
}

type authEvents struct {
	stateKeyNIDMap map[string]types.EventStateKeyNID
	state          stateEntryMap
	events         EventMap
}

// Create implements gomatrixserverlib.AuthEventProvider
func (ae *authEvents) Create() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomCreateNID), nil
}

// PowerLevels implements gomatrixserverlib.AuthEventProvider
func (ae *authEvents) PowerLevels() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomPowerLevelsNID), nil
}

// JoinRules implements gomatrixserverlib.AuthEventProvider
func (ae *authEvents) JoinRules() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomJoinRulesNID), nil
}

// Memmber implements gomatrixserverlib.AuthEventProvider
func (ae *authEvents) Member(stateKey string) (*gomatrixserverlib.Event, error) {
	return ae.lookupEvent(types.MRoomMemberNID, stateKey), nil
}

// ThirdPartyInvite implements gomatrixserverlib.AuthEventProvider
func (ae *authEvents) ThirdPartyInvite(stateKey string) (*gomatrixserverlib.Event, error) {
	return ae.lookupEvent(types.MRoomThirdPartyInviteNID, stateKey), nil
}

func (ae *authEvents) lookupEventWithEmptyStateKey(typeNID types.EventTypeNID) *gomatrixserverlib.Event {
	eventNID, ok := ae.state.lookup(types.StateKeyTuple{
		EventTypeNID:     typeNID,
		EventStateKeyNID: types.EmptyStateKeyNID,
	})
	if !ok {
		return nil
	}
	event, ok := ae.events.Lookup(eventNID)
	if !ok {
		return nil
	}
	return event.Event
}

func (ae *authEvents) lookupEvent(typeNID types.EventTypeNID, stateKey string) *gomatrixserverlib.Event {
	stateKeyNID, ok := ae.stateKeyNIDMap[stateKey]
	if !ok {
		return nil
	}
	eventNID, ok := ae.state.lookup(types.StateKeyTuple{
		EventTypeNID:     typeNID,
		EventStateKeyNID: stateKeyNID,
	})
	if !ok {
		return nil
	}
	event, ok := ae.events.Lookup(eventNID)
	if !ok {
		return nil
	}
	return event.Event
}

// loadAuthEvents loads the events needed for authentication from the supplied room state.
func loadAuthEvents(
	ctx context.Context,
	db state.StateResolutionStorage,
	needed gomatrixserverlib.StateNeeded,
	state []types.StateEntry,
) (result authEvents, err error) {
	// Look up the numeric IDs for the state keys needed for auth.
	var neededStateKeys []string
	neededStateKeys = append(neededStateKeys, needed.Member...)
	neededStateKeys = append(neededStateKeys, needed.ThirdPartyInvite...)
	if result.stateKeyNIDMap, err = db.EventStateKeyNIDs(ctx, neededStateKeys); err != nil {
		return
	}

	// Load the events we need.
	result.state = state
	var eventNIDs []types.EventNID
	keyTuplesNeeded := stateKeyTuplesNeeded(result.stateKeyNIDMap, needed)
	for _, keyTuple := range keyTuplesNeeded {
		eventNID, ok := result.state.lookup(keyTuple)
		if ok {
			eventNIDs = append(eventNIDs, eventNID)
		}
	}
	if result.events, err = db.Events(ctx, eventNIDs); err != nil {
		return
	}
	return
}

// stateKeyTuplesNeeded works out which numeric state key tuples we need to authenticate some events.
func stateKeyTuplesNeeded(
	stateKeyNIDMap map[string]types.EventStateKeyNID,
	stateNeeded gomatrixserverlib.StateNeeded,
) []types.StateKeyTuple {
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

// Map from event type, state key tuple to numeric event ID.
// Implemented using binary search on a sorted array.
type stateEntryMap []types.StateEntry

// lookup an entry in the event map.
func (m stateEntryMap) lookup(stateKey types.StateKeyTuple) (eventNID types.EventNID, ok bool) {
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
