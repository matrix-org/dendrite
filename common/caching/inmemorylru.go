package caching

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/matrix-org/gomatrixserverlib"
)

type InMemoryLRUCache struct {
	roomVersions *lru.Cache
}

func NewInMemoryLRUCache() (*InMemoryLRUCache, error) {
	roomVersionCache, rvErr := lru.New(MaxRoomVersionCacheEntries)
	if rvErr != nil {
		return nil, rvErr
	}
	return &InMemoryLRUCache{
		roomVersions: roomVersionCache,
	}, nil
}

func (c *InMemoryLRUCache) GetRoomVersion(roomID string) (gomatrixserverlib.RoomVersion, bool) {
	if c == nil {
		return "", false
	}
	val, found := c.roomVersions.Get(roomID)
	if found && val != nil {
		if roomVersion, ok := val.(gomatrixserverlib.RoomVersion); ok {
			return roomVersion, true
		}
	}
	return "", false
}

func (c *InMemoryLRUCache) StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion) {
	if c == nil {
		return
	}
	c.roomVersions.Add(roomID, roomVersion)
}
