package caching

import (
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// RoomServerNIDsCache contains the subset of functions needed for
// a roomserver NID cache.
type RoomServerEventsCache interface {
	GetRoomServerEvent(eventNID types.EventNID) (*gomatrixserverlib.Event, bool)
	StoreRoomServerEvent(eventNID types.EventNID, event *gomatrixserverlib.Event)
}

func (c Caches) GetRoomServerEvent(eventNID types.EventNID) (*gomatrixserverlib.Event, bool) {
	return c.RoomServerEvents.Get(int64(eventNID))
}

func (c Caches) StoreRoomServerEvent(eventNID types.EventNID, event *gomatrixserverlib.Event) {
	if event != nil {
		c.RoomServerEvents.Set(int64(eventNID), event)
	}
}
