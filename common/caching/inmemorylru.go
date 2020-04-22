package caching

import (
	"fmt"
	"sync"

	"github.com/golang/groupcache/lru"
	"github.com/matrix-org/gomatrixserverlib"
)

type InMemoryLRUCache struct {
	roomVersions      *lru.Cache
	roomVersionsMutex sync.RWMutex
}

func NewInMemoryLRUCache() *InMemoryLRUCache {
	return &InMemoryLRUCache{
		roomVersions: lru.New(128),
	}
}

func (c *InMemoryLRUCache) GetRoomVersion(roomID string) (gomatrixserverlib.RoomVersion, bool) {
	fmt.Println("Cache hit for", roomID)
	if c == nil {
		return "", false
	}
	c.roomVersionsMutex.RLock()
	defer c.roomVersionsMutex.RUnlock()
	val, found := c.roomVersions.Get(roomID)
	return val.(gomatrixserverlib.RoomVersion), found
}

func (c *InMemoryLRUCache) StoreRoomVersion(roomID string, roomVersion gomatrixserverlib.RoomVersion) {
	fmt.Println("Cache store for", roomID)
	if c == nil {
		return
	}
	c.roomVersionsMutex.Lock()
	defer c.roomVersionsMutex.Unlock()
	c.roomVersions.Add(roomID, roomVersion)
}
