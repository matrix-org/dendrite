package caching

import "github.com/matrix-org/gomatrixserverlib"

const (
	MaxRoomVersionCacheEntries = 128
)

type Cache interface {
	GetRoomVersion(roomId string) (gomatrixserverlib.RoomVersion, bool)
	StoreRoomVersion(roomId string, roomVersion gomatrixserverlib.RoomVersion)
}
