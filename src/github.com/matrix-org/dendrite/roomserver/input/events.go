package input

import (
	"fmt"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"sort"
)

// A RoomEventDatabase has the storage APIs needed to store a room event.
type RoomEventDatabase interface {
	// Stores a matrix room event in the database
	StoreEvent(event gomatrixserverlib.Event, authEventNIDs []int64) error
	// Lookup the state entries for a list of string event IDs
	// Returns a sorted list of state entries.
	StateEntriesForEventIDs(eventIDs []string) ([]types.StateEntry, error)
	// Lookup the numeric IDs for a list of string event state keys.
	// Returns a sorted list of state entries.
	EventStateKeyNIDs(eventStateKeys []string) ([]types.IDPair, error)
	// Lookup the Events for a list of numeric event IDs.
	// Returns a sorted list of state entries.
	Events(eventNIDs []int64) ([]types.Event, error)
}

func processRoomEvent(db RoomEventDatabase, input api.InputRoomEvent) error {
	// Parse and validate the event JSON
	event, err := gomatrixserverlib.NewEventFromUntrustedJSON(input.Event)
	if err != nil {
		return err
	}

	// Check that the event passes authentication checks.
	authEventNIDs, err := checkAuthEvents(db, event, input.AuthEventIDs)
	if err != nil {
		return err
	}

	// Store the event
	if err := db.StoreEvent(event, authEventNIDs); err != nil {
		return err
	}

	if input.Kind == api.KindOutlier {
		// For outliers we can stop after we've stored the event itself as it
		// doesn't have any associated state to store and we don't need to
		// notify anyone about it.
		return nil
	}

	// TODO:
	//  * Calcuate the state at the event if necessary.
	//  * Store the state at the event.
	//  * Update the extremities of the event graph for the room
	//  * Caculate the new current state for the room if the forward extremities have changed.
	//  * Work out the delta between the new current state and the previous current state.
	//  * Work out the visibility of the event.
	//  * Write a message to the output logs containing:
	//      - The event itself
	//      - The visiblity of the event, i.e. who is allowed to see the event.
	//      - The changes to the current state of the room.
	panic("Not implemented")
}

// checkAuthEvents checks that the event passes authentication checks
// Returns the numeric IDs for the auth events.
func checkAuthEvents(db RoomEventDatabase, event gomatrixserverlib.Event, authEventIDs []string) ([]int64, error) {
	// Grab the numeric IDs for the supplied auth state events from the database.
	authStateEntries, err := db.StateEntriesForEventIDs(authEventIDs)
	if err != nil {
		return nil, err
	}
	if len(authStateEntries) < len(authEventIDs) {
		return nil, fmt.Errorf("input: Some of the auth event IDs were missing from the database")
	}

	// TODO: check for duplicate state keys here.

	// Work out which of the state events we actaully need.
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
	stateNIDMap idMap
	state       stateEntryMap
	events      eventMap
}

func (ae *authEvents) Create() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomCreateNID), nil
}

func (ae *authEvents) PowerLevels() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomPowerLevelsNID), nil
}

func (ae *authEvents) JoinRules() (*gomatrixserverlib.Event, error) {
	return ae.lookupEventWithEmptyStateKey(types.MRoomJoinRulesNID), nil
}

func (ae *authEvents) Member(stateKey string) (*gomatrixserverlib.Event, error) {
	return ae.lookupEvent(types.MRoomMemberNID, stateKey), nil
}

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
	stateKeyNID, ok := ae.stateNIDMap.lookup(stateKey)
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
	// Lookup the numeric IDs for the state keys
	var eventStateKeys []string
	eventStateKeys = append(eventStateKeys, needed.Member...)
	eventStateKeys = append(eventStateKeys, needed.ThirdPartyInvite...)
	stateKeyNIDs, err := db.EventStateKeyNIDs(eventStateKeys)
	if err != nil {
		return
	}
	result.stateNIDMap = newIDMap(stateKeyNIDs)

	// Load the events we need.
	keysNeeded := stateKeysNeeded(result.stateNIDMap, needed)
	var eventNIDs []int64
	result.state = newStateEntryMap(state)
	for _, keyNeeded := range keysNeeded {
		eventNID, ok := result.state.lookup(keyNeeded)
		if ok {
			eventNIDs = append(eventNIDs, eventNID)
		}
	}
	events, err := db.Events(eventNIDs)
	if err != nil {
		return
	}
	result.events = newEventMap(events)
	return
}

// stateKeysNeeded works out which numeric state keys we need to authenticate some events.
func stateKeysNeeded(stateNIDMap idMap, stateNeeded gomatrixserverlib.StateNeeded) []types.StateKeyTuple {
	var keys []types.StateKeyTuple
	if stateNeeded.Create {
		keys = append(keys, types.StateKeyTuple{types.MRoomCreateNID, types.EmptyStateKeyNID})
	}
	if stateNeeded.PowerLevels {
		keys = append(keys, types.StateKeyTuple{types.MRoomPowerLevelsNID, types.EmptyStateKeyNID})
	}
	if stateNeeded.JoinRules {
		keys = append(keys, types.StateKeyTuple{types.MRoomJoinRulesNID, types.EmptyStateKeyNID})
	}
	for _, member := range stateNeeded.Member {
		stateKeyNID, ok := stateNIDMap.lookup(member)
		if ok {
			keys = append(keys, types.StateKeyTuple{types.MRoomMemberNID, stateKeyNID})
		}
	}
	for _, token := range stateNeeded.ThirdPartyInvite {
		stateKeyNID, ok := stateNIDMap.lookup(token)
		if ok {
			keys = append(keys, types.StateKeyTuple{types.MRoomThirdPartyInviteNID, stateKeyNID})
		}
	}
	return keys
}

type idMap map[string]int64

func newIDMap(ids []types.IDPair) idMap {
	result := make(map[string]int64)
	for _, pair := range ids {
		result[pair.ID] = pair.NID
	}
	return idMap(result)
}

// lookup an entry in the id map.
func (m idMap) lookup(id string) (nid int64, ok bool) {
	// Use a hash map here.
	// We could use binary search here like we do for the maps below as it
	// would be faster for small lists.
	// However the benefits of binary search aren't as strong here and it's
	// possible that we could encounter sets of pathological strings since
	// the state keys are ultimately controlled by user input.
	nid, ok = map[string]int64(m)[id]
	return
}

type stateEntryMap []types.StateEntry

// newStateEntryMap creates a map from a sorted list of state entries.
func newStateEntryMap(stateEntries []types.StateEntry) stateEntryMap {
	return stateEntryMap(stateEntries)
}

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

type eventMap []types.Event

// newEventMap creates a map from a sorted list of events.
func newEventMap(events []types.Event) eventMap {
	return eventMap(events)
}

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
