package caching

import "github.com/matrix-org/gomatrixserverlib"

const (
	RoomVersionMaxCacheEntries = 128
	ServerKeysMaxCacheEntries  = 128
)

type ImmutableCache interface {
	GetRoomVersion(roomId string) (gomatrixserverlib.RoomVersion, bool)
	StoreRoomVersion(roomId string, roomVersion gomatrixserverlib.RoomVersion)
	GetServerKey(request gomatrixserverlib.PublicKeyLookupRequest) (gomatrixserverlib.PublicKeyLookupResult, bool)
	StoreServerKey(request gomatrixserverlib.PublicKeyLookupRequest, response gomatrixserverlib.PublicKeyLookupResult)
}
