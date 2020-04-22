package caching

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru"
	"github.com/matrix-org/gomatrixserverlib"
)

type ImmutableInMemoryLRUCache struct {
	roomVersions *lru.Cache
}

func NewImmutableInMemoryLRUCache() (*ImmutableInMemoryLRUCache, error) {
	roomVersionCache, rvErr := lru.New(RoomVersionMaxCacheEntries)
	if rvErr != nil {
		return nil, rvErr
	}
	return &ImmutableInMemoryLRUCache{
		roomVersions: roomVersionCache,
	}, nil
}

func checkForInvalidMutation(cache *lru.Cache, key string, value interface{}) {
	if peek, ok := cache.Peek(key); ok && peek != value {
		panic(fmt.Sprintf("invalid use of immutable cache tries to mutate existing value of %q", key))
	}
}

func (c *ImmutableInMemoryLRUCache) GetRoomVersion(roomID string) (gomatrixserverlib.RoomVersion, bool) {
	val, found := c.roomVersions.Get(roomID)
	if found && val != nil {
		if roomVersion, ok := val.(gomatrixserverlib.RoomVersion); ok {
			return roomVersion, true
		}
	}
	return "", false
}

func (c *ImmutableInMemoryLRUCache) StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion) {
	checkForInvalidMutation(c.roomVersions, roomID, roomVersion)
	c.roomVersions.Add(roomID, roomVersion)
}
