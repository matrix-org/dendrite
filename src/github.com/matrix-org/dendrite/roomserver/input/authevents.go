package input

import (
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"sort"
)

// checkAuthEvents checks that the event passes authentication checks
// Returns the numeric IDs for the auth events.
func checkAuthEvents(db RoomEventDatabase, event gomatrixserverlib.Event, authEventIDs []string) ([]int64, error) {
	// Grab the numeric IDs for the supplied auth state events from the database.
	authStateEntries, err := db.StateEntriesForEventIDs(authEventIDs)
	if err != nil {
		return nil, err
	}
	// TODO: check for duplicate state keys here.

	// Work out which of the state events we actually need.
	stateNeeded := gomatrixserverlib.StateNeededForAuth([]gomatrixserverlib.Event{event})

	// Load the actual auth events from the database.
	authEvents, err := loadAuthEvents(db, stateNeeded, authStateEntries)
	if err != nil {
		return nil, err
	}

	// Check if the event is allowed.
	if err = gomatrixserverlib.Allowed(event, &authEvents); err != nil {
		return nil, err
	}

	// Return the numeric IDs for the auth events.
	result := make([]int64, len(authStateEntries))
	for i := range authStateEntries {
		result[i] = authStateEntries[i].EventNID
	}
	return result, nil
}

type authEvents struct {
	stateKeyNIDMap map[string]int64
	state          stateEntryMap
	events         eventMap
}

// Create implements gomatrixserverlib.AuthEvents
func (ae *authEvents) Create() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomCreateNID), nil
}

// PowerLevels implements gomatrixserverlib.AuthEvents
func (ae *authEvents) PowerLevels() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomPowerLevelsNID), nil
}

// JoinRules implements gomatrixserverlib.AuthEvents
func (ae *authEvents) JoinRules() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomJoinRulesNID), nil
}

// Memmber implements gomatrixserverlib.AuthEvents
func (ae *authEvents) Member(stateKey string) (*gomatrixserverlib.Event, error) {
	return ae.lookupEvent(types.MRoomMemberNID, stateKey), nil
}

// ThirdPartyInvite implements gomatrixserverlib.AuthEvents
func (ae *authEvents) ThirdPartyInvite(stateKey string) (*gomatrixserverlib.Event, error) {
	return ae.lookupEvent(types.MRoomThirdPartyInviteNID, stateKey), nil
}

func (ae *authEvents) lookupEventWithEmptyStateKey(typeNID int64) *gomatrixserverlib.Event {
	eventNID, ok := ae.state.lookup(types.StateKeyTuple{typeNID, types.EmptyStateKeyNID})
	if !ok {
		return nil
	}
	event, ok := ae.events.lookup(eventNID)
	if !ok {
		return nil
	}
	return &event.Event
}

func (ae *authEvents) lookupEvent(typeNID int64, stateKey string) *gomatrixserverlib.Event {
	stateKeyNID, ok := ae.stateKeyNIDMap[stateKey]
	if !ok {
		return nil
	}
	eventNID, ok := ae.state.lookup(types.StateKeyTuple{typeNID, stateKeyNID})
	if !ok {
		return nil
	}
	event, ok := ae.events.lookup(eventNID)
	if !ok {
		return nil
	}
	return &event.Event
}

// loadAuthEvents loads the events needed for authentication from the supplied room state.
func loadAuthEvents(
	db RoomEventDatabase,
	needed gomatrixserverlib.StateNeeded,
	state []types.StateEntry,
) (result authEvents, err error) {
	// Lookup the numeric IDs for the state keys needed for auth.
	var neededStateKeys []string
	neededStateKeys = append(neededStateKeys, needed.Member...)
	neededStateKeys = append(neededStateKeys, needed.ThirdPartyInvite...)
	if result.stateKeyNIDMap, err = db.EventStateKeyNIDs(neededStateKeys); err != nil {
		return
	}

	// Load the events we need.
	result.state = state
	var eventNIDs []int64
	keyTuplesNeeded := stateKeyTuplesNeeded(result.stateKeyNIDMap, needed)
	for _, keyTuple := range keyTuplesNeeded {
		eventNID, ok := result.state.lookup(keyTuple)
		if ok {
			eventNIDs = append(eventNIDs, eventNID)
		}
	}
	if result.events, err = db.Events(eventNIDs); err != nil {
		return
	}
	return
}

// stateKeyTuplesNeeded works out which numeric state key tuples we need to authenticate some events.
func stateKeyTuplesNeeded(stateKeyNIDMap map[string]int64, stateNeeded gomatrixserverlib.StateNeeded) []types.StateKeyTuple {
	var keyTuples []types.StateKeyTuple
	if stateNeeded.Create {
		keyTuples = append(keyTuples, types.StateKeyTuple{types.MRoomCreateNID, types.EmptyStateKeyNID})
	}
	if stateNeeded.PowerLevels {
		keyTuples = append(keyTuples, types.StateKeyTuple{types.MRoomPowerLevelsNID, types.EmptyStateKeyNID})
	}
	if stateNeeded.JoinRules {
		keyTuples = append(keyTuples, types.StateKeyTuple{types.MRoomJoinRulesNID, types.EmptyStateKeyNID})
	}
	for _, member := range stateNeeded.Member {
		stateKeyNID, ok := stateKeyNIDMap[member]
		if ok {
			keyTuples = append(keyTuples, types.StateKeyTuple{types.MRoomMemberNID, stateKeyNID})
		}
	}
	for _, token := range stateNeeded.ThirdPartyInvite {
		stateKeyNID, ok := stateKeyNIDMap[token]
		if ok {
			keyTuples = append(keyTuples, types.StateKeyTuple{types.MRoomThirdPartyInviteNID, stateKeyNID})
		}
	}
	return keyTuples
}

// Map from event type, state key tuple to numeric event ID.
// Implemented using binary search on a sorted array.
type stateEntryMap []types.StateEntry

// lookup an entry in the event map.
func (m stateEntryMap) lookup(stateKey types.StateKeyTuple) (eventNID int64, ok bool) {
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
type eventMap []types.Event

// lookup an entry in the event map.
func (m eventMap) lookup(eventNID int64) (event *types.Event, ok bool) {
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
