package caching

import "github.com/matrix-org/dendrite/roomserver/types"

// EventStateKeyCache contains the subset of functions needed for
// a room event state key cache.
type EventStateKeyCache interface {
	GetEventStateKey(eventStateKeyNID types.EventStateKeyNID) (string, bool)
	StoreEventStateKey(eventStateKeyNID types.EventStateKeyNID, eventStateKey string)
}

func (c Caches) GetEventStateKey(eventStateKeyNID types.EventStateKeyNID) (string, bool) {
	return c.RoomServerStateKeys.Get(eventStateKeyNID)
}

func (c Caches) StoreEventStateKey(eventStateKeyNID types.EventStateKeyNID, eventStateKey string) {
	c.RoomServerStateKeys.Set(eventStateKeyNID, eventStateKey)
}
