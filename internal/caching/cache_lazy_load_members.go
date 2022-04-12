package caching

import (
	"fmt"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

const (
	LazyLoadCacheName       = "lazy_load_members"
	LazyLoadCacheMaxEntries = 128
	LazyLoadCacheMutable    = true
	LazyLoadCacheMaxAge     = time.Minute * 30
)

type LazyLoadCache struct {
	*InMemoryLRUCachePartition
}

// NewLazyLoadCache creates a new InMemoryLRUCachePartition.
func NewLazyLoadCache() (*LazyLoadCache, error) {
	cache, err := NewInMemoryLRUCachePartition(
		LazyLoadCacheName,
		LazyLoadCacheMutable,
		LazyLoadCacheMaxEntries,
		LazyLoadCacheMaxAge,
		false,
	)
	if err != nil {
		return nil, err
	}
	go cacheCleaner(cache)
	return &LazyLoadCache{cache}, err
}

func (c *LazyLoadCache) StoreLazyLoadedMembers(reqUser, deviceID, roomID, userID string, event *gomatrixserverlib.HeaderedEvent) {
	cacheKey := fmt.Sprintf("%s/%s/%s/%s", reqUser, deviceID, roomID, userID)
	c.Set(cacheKey, event)
}

func (c *LazyLoadCache) GetLazyLoadedMembers(reqUser, deviceID, roomID, userID string) (*gomatrixserverlib.HeaderedEvent, bool) {
	cacheKey := fmt.Sprintf("%s/%s/%s/%s", reqUser, deviceID, roomID, userID)
	val, ok := c.Get(cacheKey)
	if !ok {
		return nil, ok
	}
	return val.(*gomatrixserverlib.HeaderedEvent), ok
}
