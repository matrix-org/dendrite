package caching

import (
	"github.com/matrix-org/dendrite/roomserver/types"
)

const (
	RoomInfoCacheName       = "room_info"
	RoomInfoCacheMaxEntries = 1024
	RoomInfoCacheMutable    = false
)

// RoomVersionsCache contains the subset of functions needed for
// a room version cache.
type RoomInfoCache interface {
	GetRoomInfo(roomID string) (roomVersion types.RoomInfo, ok bool)
	StoreRoomInfo(roomID string, roomVersion types.RoomInfo)
}

func (c Caches) GetRoomInfo(roomID string) (types.RoomInfo, bool) {
	val, found := c.RoomServerRoomInfo.Get(roomID)
	if found && val != nil {
		if roomInfo, ok := val.(types.RoomInfo); ok {
			return roomInfo, true
		}
	}
	return types.RoomInfo{}, false
}

func (c Caches) StoreRoomInfo(roomID string, roomInfo types.RoomInfo) {
	c.RoomServerRoomInfo.Set(roomID, roomInfo)
}
