package caching

import (
	"github.com/matrix-org/dendrite/roomserver/types"
)

const (
	RoomInfoCacheName       = "roominfo"
	RoomInfoCacheMaxEntries = 1024
	RoomInfoCacheMutable    = true
)

// RoomInfosCache contains the subset of functions needed for
// a room Info cache.
type RoomInfoCache interface {
	GetRoomInfo(roomID string) (roomInfo types.RoomInfo, ok bool)
	StoreRoomInfo(roomID string, roomInfo types.RoomInfo)
}

func (c Caches) GetRoomInfo(roomID string) (types.RoomInfo, bool) {
	val, found := c.RoomInfos.Get(roomID)
	if found && val != nil {
		if roomInfo, ok := val.(types.RoomInfo); ok {
			return roomInfo, true
		}
	}
	return types.RoomInfo{}, false
}

func (c Caches) StoreRoomInfo(roomID string, roomInfo types.RoomInfo) {
	c.RoomInfos.Set(roomID, roomInfo)
}
