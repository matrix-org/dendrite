package caching

import "github.com/matrix-org/gomatrixserverlib"

const (
	RoomVersionCacheName       = "room_versions"
	RoomVersionCacheMaxEntries = 1024
	RoomVersionCacheMutable    = false
	RoomVersionCacheMaxAge     = CacheNoMaxAge
)

// RoomVersionsCache contains the subset of functions needed for
// a room version cache.
type RoomVersionCache interface {
	GetRoomVersion(roomID string) (roomVersion gomatrixserverlib.RoomVersion, ok bool)
	StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion)
}

func (c Caches) GetRoomVersion(roomID string) (gomatrixserverlib.RoomVersion, bool) {
	val, found := c.RoomVersions.Get(roomID)
	if found && val != nil {
		if roomVersion, ok := val.(gomatrixserverlib.RoomVersion); ok {
			return roomVersion, true
		}
	}
	return "", false
}

func (c Caches) StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion) {
	c.RoomVersions.Set(roomID, roomVersion)
}
