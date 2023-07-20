package caching

import (
	"github.com/matrix-org/dendrite/roomserver/types"
)

type RoomServerCaches interface {
	RoomServerNIDsCache
	RoomVersionCache
	RoomServerEventsCache
	RoomHierarchyCache
	EventStateKeyCache
	EventTypeCache
}

// RoomServerNIDsCache contains the subset of functions needed for
// a roomserver NID cache.
type RoomServerNIDsCache interface {
	GetRoomServerRoomID(roomNID types.RoomNID) (string, bool)
	// StoreRoomServerRoomID stores roomNID -> roomID and roomID -> roomNID
	StoreRoomServerRoomID(roomNID types.RoomNID, roomID string)
	GetRoomServerRoomNID(roomID string) (types.RoomNID, bool)
}

func (c Caches) GetRoomServerRoomID(roomNID types.RoomNID) (string, bool) {
	return c.RoomServerRoomIDs.Get(roomNID)
}

// StoreRoomServerRoomID stores roomNID -> roomID and roomID -> roomNID
func (c Caches) StoreRoomServerRoomID(roomNID types.RoomNID, roomID string) {
	c.RoomServerRoomNIDs.Set(roomID, roomNID)
	c.RoomServerRoomIDs.Set(roomNID, roomID)
}

func (c Caches) GetRoomServerRoomNID(roomID string) (types.RoomNID, bool) {
	return c.RoomServerRoomNIDs.Get(roomID)
}
