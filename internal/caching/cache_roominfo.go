package caching

import (
	"time"

	"github.com/matrix-org/dendrite/roomserver/types"
)

// WARNING: This cache is mutable because it's entirely possible that
// the IsStub or StateSnaphotNID fields can change, even though the
// room version and room NID fields will not. This is only safe because
// the RoomInfoCache is used ONLY within the roomserver and because it
// will be kept up-to-date by the latest events updater. It MUST NOT be
// used from other components as we currently have no way to invalidate
// the cache in downstream components.

const (
	RoomInfoCacheName       = "roominfo"
	RoomInfoCacheMaxEntries = 1024
	RoomInfoCacheMutable    = true
	RoomInfoCacheMaxAge     = time.Minute * 5
)

// RoomInfosCache contains the subset of functions needed for
// a room Info cache. It must only be used from the roomserver only
// It is not safe for use from other components.
type RoomInfoCache interface {
	GetRoomInfo(roomID string) (roomInfo types.RoomInfo, ok bool)
	StoreRoomInfo(roomID string, roomInfo types.RoomInfo)
}

// GetRoomInfo must only be called from the roomserver only. It is not
// safe for use from other components.
func (c Caches) GetRoomInfo(roomID string) (types.RoomInfo, bool) {
	val, found := c.RoomInfos.Get(roomID)
	if found && val != nil {
		if roomInfo, ok := val.(types.RoomInfo); ok {
			return roomInfo, true
		}
	}
	return types.RoomInfo{}, false
}

// StoreRoomInfo must only be called from the roomserver only. It is not
// safe for use from other components.
func (c Caches) StoreRoomInfo(roomID string, roomInfo types.RoomInfo) {
	c.RoomInfos.Set(roomID, roomInfo)
}
