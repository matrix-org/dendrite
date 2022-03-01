package caching

import (
	"strconv"

	"github.com/matrix-org/dendrite/roomserver/types"
)

const (
	RoomServerRoomIDsCacheName       = "roomserver_room_ids"
	RoomServerRoomIDsCacheMaxEntries = 1024
	RoomServerRoomIDsCacheMutable    = false
	RoomServerRoomIDsCacheMaxAge     = CacheNoMaxAge
)

type RoomServerCaches interface {
	RoomServerNIDsCache
	RoomVersionCache
	RoomInfoCache
}

// RoomServerNIDsCache contains the subset of functions needed for
// a roomserver NID cache.
type RoomServerNIDsCache interface {
	GetRoomServerRoomID(roomNID types.RoomNID) (string, bool)
	StoreRoomServerRoomID(roomNID types.RoomNID, roomID string)
}

func (c Caches) GetRoomServerRoomID(roomNID types.RoomNID) (string, bool) {
	val, found := c.RoomServerRoomIDs.Get(strconv.Itoa(int(roomNID)))
	if found && val != nil {
		if roomID, ok := val.(string); ok {
			return roomID, true
		}
	}
	return "", false
}

func (c Caches) StoreRoomServerRoomID(roomNID types.RoomNID, roomID string) {
	c.RoomServerRoomIDs.Set(strconv.Itoa(int(roomNID)), roomID)
}
