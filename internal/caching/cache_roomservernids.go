package caching

import (
	"github.com/matrix-org/dendrite/roomserver/types"
)

type RoomServerCaches interface {
	RoomServerNIDsCache
	RoomVersionCache
	RoomInfoCache
	RoomServerEventsCache
}

// RoomServerNIDsCache contains the subset of functions needed for
// a roomserver NID cache.
type RoomServerNIDsCache interface {
	GetRoomServerRoomID(roomNID types.RoomNID) (string, bool)
	StoreRoomServerRoomID(roomNID types.RoomNID, roomID string)
}

func (c Caches) GetRoomServerRoomID(roomNID types.RoomNID) (string, bool) {
	return c.RoomServerRoomIDs.Get(int64(roomNID))
}

func (c Caches) StoreRoomServerRoomID(roomNID types.RoomNID, roomID string) {
	c.RoomServerRoomIDs.Set(int64(roomNID), roomID)
}
