package caching

import (
	"github.com/element-hq/dendrite/roomserver/types"
)

// RoomServerEventsCache contains the subset of functions needed for
// a roomserver event cache.
type RoomServerEventsCache interface {
	GetRoomServerEvent(eventNID types.EventNID) (*types.HeaderedEvent, bool)
	StoreRoomServerEvent(eventNID types.EventNID, event *types.HeaderedEvent)
	InvalidateRoomServerEvent(eventNID types.EventNID)
}

func (c Caches) GetRoomServerEvent(eventNID types.EventNID) (*types.HeaderedEvent, bool) {
	return c.RoomServerEvents.Get(int64(eventNID))
}

func (c Caches) StoreRoomServerEvent(eventNID types.EventNID, event *types.HeaderedEvent) {
	c.RoomServerEvents.Set(int64(eventNID), event)
}

func (c Caches) InvalidateRoomServerEvent(eventNID types.EventNID) {
	c.RoomServerEvents.Unset(int64(eventNID))
}
