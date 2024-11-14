package caching

import "github.com/element-hq/dendrite/roomserver/types"

// EventStateKeyCache contains the subset of functions needed for
// a room event state key cache.
type EventStateKeyCache interface {
	GetEventStateKey(eventStateKeyNID types.EventStateKeyNID) (string, bool)
	StoreEventStateKey(eventStateKeyNID types.EventStateKeyNID, eventStateKey string)
	GetEventStateKeyNID(eventStateKey string) (types.EventStateKeyNID, bool)
}

func (c Caches) GetEventStateKey(eventStateKeyNID types.EventStateKeyNID) (string, bool) {
	return c.RoomServerStateKeys.Get(eventStateKeyNID)
}

func (c Caches) StoreEventStateKey(eventStateKeyNID types.EventStateKeyNID, eventStateKey string) {
	c.RoomServerStateKeys.Set(eventStateKeyNID, eventStateKey)
	c.RoomServerStateKeyNIDs.Set(eventStateKey, eventStateKeyNID)
}

func (c Caches) GetEventStateKeyNID(eventStateKey string) (types.EventStateKeyNID, bool) {
	return c.RoomServerStateKeyNIDs.Get(eventStateKey)
}

type EventTypeCache interface {
	GetEventTypeKey(eventType string) (types.EventTypeNID, bool)
	StoreEventTypeKey(eventTypeNID types.EventTypeNID, eventType string)
}

func (c Caches) StoreEventTypeKey(eventTypeNID types.EventTypeNID, eventType string) {
	c.RoomServerEventTypeNIDs.Set(eventType, eventTypeNID)
	c.RoomServerEventTypes.Set(eventTypeNID, eventType)
}

func (c Caches) GetEventTypeKey(eventType string) (types.EventTypeNID, bool) {
	return c.RoomServerEventTypeNIDs.Get(eventType)
}
