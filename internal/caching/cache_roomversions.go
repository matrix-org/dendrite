package caching

import "github.com/matrix-org/gomatrixserverlib"

// RoomVersionsCache contains the subset of functions needed for
// a room version cache.
type RoomVersionCache interface {
	GetRoomVersion(roomID string) (roomVersion gomatrixserverlib.RoomVersion, ok bool)
	StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion)
}

func (c Caches) GetRoomVersion(roomID string) (gomatrixserverlib.RoomVersion, bool) {
	return c.RoomVersions.Get(roomID)
}

func (c Caches) StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion) {
	c.RoomVersions.Set(roomID, roomVersion)
}
