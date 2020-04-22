package caching

import "github.com/matrix-org/gomatrixserverlib"

type Cache interface {
	GetRoomVersion(roomId string) (gomatrixserverlib.RoomVersion, bool)
	StoreRoomVersion(roomId string, roomVersion gomatrixserverlib.RoomVersion)
}
