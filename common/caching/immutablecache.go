package caching

import "github.com/matrix-org/gomatrixserverlib"

const (
	RoomVersionMaxCacheEntries = 128
	ServerKeysMaxCacheEntries  = 128
)

type ImmutableCache interface {
	gomatrixserverlib.KeyCache
	GetRoomVersion(roomId string) (gomatrixserverlib.RoomVersion, bool)
	StoreRoomVersion(roomId string, roomVersion gomatrixserverlib.RoomVersion)
}
