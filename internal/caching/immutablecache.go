package caching

import (
	"github.com/matrix-org/gomatrixserverlib"
)

const (
	RoomVersionMaxCacheEntries = 1024
	ServerKeysMaxCacheEntries  = 1024
)

type ImmutableCache interface {
	GetRoomVersion(roomId string) (gomatrixserverlib.RoomVersion, bool)
	StoreRoomVersion(roomId string, roomVersion gomatrixserverlib.RoomVersion)
	GetServerKey(request gomatrixserverlib.PublicKeyLookupRequest) (gomatrixserverlib.PublicKeyLookupResult, bool)
	StoreServerKey(request gomatrixserverlib.PublicKeyLookupRequest, response gomatrixserverlib.PublicKeyLookupResult)
}
